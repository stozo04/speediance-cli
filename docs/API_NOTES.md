# Speediance API — Data Model & Telemetry Notes

These notes document empirical observations from the Speediance cloud API on a live
Gym Monster 1 account (app v4.4.0, device_type=1, Global region, observed November 2026).
These notes document data quirks, wire-format semantics, and traps observed in the
reverse-engineered API to help callers interpret sessions and avoid silent data corruption.
Most of them fail *silently*, which is why they are documented here.

---

## 1. `Versioncode` Gates Content (Not Cosmetic)

*(Note: Addressed in speediance-cli v2.0.2 / PR #44 by raising default `appVersionCode` to `40400`)*

The HTTP client sends a `Versioncode` header indicating the client app release.
**Speediance gates newer content behind newer version codes.** Any request whose response
contains gated exercises was observed to be rejected outright:

```json
{
  "code": 98,
  "message": "Please upgrade the APP version in System Setting"
}
```

For example, Vita exercises shipped in app **v4.4.0**, so `Versioncode` must be **>= 40400**.
Observed by bisection against the live API: `40399` was blocked, `40400` was accepted.

The critical trap is the *silent* half: the workout-detail endpoint returns an explicit code 98 error,
but the **library endpoint simply omits the gated exercises** (e.g. 914 results at 40304 vs 921 at 40400,
with code 0 and no error). If exercises known to exist in the app are missing from the catalog,
check `Versioncode` first.

## 2. `completionMethod` Decides What `targetCount` Means

On exercise objects in session details, always read `completionMethod` before interpreting reps or duration:

| Value | Set completes when… | `targetCount` represents | `finishedCount` represents |
|---|---|---|---|
| `1` | A rep target is hit | **Reps** | Reps completed |
| `0` | A duration elapses | **Seconds** | Always `0` (no reps counted) |
| `2` | A duration elapses (Row/Ski) | **Seconds** | Always `0` |
| `5` | A duration elapses (timed/Vita) | **Seconds** | **Reps achieved inside the window** |

A timed set reading `finishedCount: 15, targetCount: 20` with `completionMethod: 5` indicates
**15 reps achieved within a 20-second window** — not an incomplete or failed set.

Per set, `time` is the seconds actually spent working. `time: 0` with `finishedCount: 0` was
consistently observed when a set was skipped.

## 3. Vita Intensity is a Level (in `level`), Not Resistance

On observed Vita exercises (`dataStatType: 6`), intensity was recorded in a **`level`** CSV,
while `weights` was sent as all zeros:

```
Vita Twist: setsAndReps="20,20,20,20", level="10,12,14,16", weights="0,0,0,0"
```

Consumers must read `level` when `dataStatType == 6`, or every set will appear to have an intensity of 0.

**Observed behavior on level ranges:** In test and device-authored workouts, levels 10, 12, 14, and 16
were stored and returned verbatim by the server without clamping. While `recommendedLevel` in the library
is typically around 5, the API did not reject or clamp higher level values.

## 4. `weights` is Cable Tension on Dual-Cable Exercises, Not Resistance

On observed single-cable exercises, the top-level `weights` array matched the populated side array.
On dual-cable exercises, however, the top-level `weights` array was observed to be a derived force series
reflecting total cable tension (which spikes during acceleration):

```
Dual-cable movement, set 1:
  leftWeights  (13 items): [7.5, 11.5, 11.5, 12, 12, 12, ...]              -> 7.5 to 12
  rightWeights (14 items): [7.5, 11.0, 11.5, 12, 12.5, 12.5, ...]            -> 7.5 to 12.5
  weights      (27 items): [7.5, 7.5, 48.5, 47.0, 47.5, 48.5, 12, 30.5, ...] -> 7.5 to 48.5
```

The top-level `weights` array holds values (e.g. 48.5, 47.0) present in *neither* side array.
Interpreting `weights` as resistance makes it appear as though load dropped from 48.5 → 11.5,
when resistance actually ramped 7.5 → 12.

**Rule:** Always use `leftWeights` and `rightWeights` for true per-handle resistance.
Treat top-level `weights` only as a fallback for single-cable exercises.

## 5. Per-Rep Telemetry Arrays are Ragged — Do Not Zip by Index

In `finishedReps[].trainingInfoDetail`, each channel array (`leftWatts`, `rightWatts`,
`leftAmplitudes`, `rightAmplitudes`, `leftRopeSpeeds`, etc.) carries telemetry per rep.
However, **channels do not necessarily share the same array length within a single set**:

```
leftAmplitudes  -> 13 values
leftWatts       -> 12 values
rightAmplitudes -> 14 values
rightWatts      -> 13 values
```

Positionally zipping channels by index (`plot(watts[i], weights[i])`) silently shifts telemetry
across repetitions. Treat each series independently, scaled over its own length.

## 6. Unilateral Exercises: Sides Alternate by Parity

On unilateral exercises (`isLeftRight == 1`), the Speediance API was observed to assign sides by position index:
**even index = Left (1), odd index = Right (2)**, stored as `leftRight: "1,2,1,2"`.

Prescribing different loads per side requires creating paired sets for each round:
two physical working sets become four list entries.

## 7. Account Units: No Server-Side Conversion Occurs

In round-trip tests, the API stored and returned workout weights in the unit configured on the user's account
(`unit: 1` = lbs, `unit: 0` = kg):
- **On Read:** Raw numbers represent the account's native unit. `35` on an imperial account is 35 lbs.
- **On Write:** Custom template weights are stored verbatim without automatic conversion.
  If an author pushes kilogram values to an account expecting pounds, the machine will load
  approximately 45% of the intended resistance.

The sole exception is the exercise library catalog's `recommendedWeight`, which is always returned in kg.

## 8. Presets vs Custom Weights

In observed template creation payloads:
Template payloads accept RM-based presets (`presetId != -1`) or custom weights (`presetId: -1`).
When using RM-based presets, the target percentage is stored in `counterweight2`, while `weights`
holds 0.
