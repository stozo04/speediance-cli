"""Build & push Speediance custom training templates, and fetch the exercise library.

Ports the customTrainingTemplate creation logic from the MIT-licensed
UnofficialSpeedianceWorkoutManager (hbui3). We use the custom-weight path
(explicit kg per set), which is what coach-authored plans use.

A *plan* is JSON in this shape (same format as hbui3's llm_prompt):
{
  "name": "Week 1 - Push",
  "exercises": [
    {"id": 304, "title": "Standing Dual-Handle Chest Press",
     "sets": [{"reps": 15, "weight": 20, "mode": 1, "rest": 75}, ...]}
  ]
}
weight is in KG. mode 1=Standard. rest in seconds.
"""
import json
import logging

logger = logging.getLogger("speediance")

# API weight unit factor (kg -> internal), per hbui3's reverse-engineering.
KG_TO_API = 2.2


# ---- exercise library -------------------------------------------------
def fetch_library(client, device_type=1):
    """Return [{id, name, muscle}] for the user's device (1=Gym Monster)."""
    cats = client._get(f"/app/actionLibraryTab/list?deviceType={device_type}")
    out, seen = [], set()
    for cat in (cats.get("data") or []):
        tab_id = cat.get("id")
        groups = client._get(
            f"/app/actionLibraryGroup/trainingPartGroup?tabId={tab_id}&deviceTypeList={device_type}"
        )
        for muscle in (groups.get("data") or []):
            mname = muscle.get("name") or muscle.get("trainingPartName") or ""
            for action in muscle.get("actionLibraryGroupList", []):
                aid = action.get("id")
                if aid in seen:
                    continue
                seen.add(aid)
                out.append({"id": aid, "name": action.get("name", ""), "muscle": mname})
    return out


# ---- template creation ------------------------------------------------
def _is_unilateral(client, group_id):
    d = client._get(f"/app/actionLibraryGroup/{group_id}?isDisplay=1")
    return (d.get("data") or {}).get("isLeftRight") == 1


def _resolve_variant_ids(client, group_ids):
    """groupId -> actionLibraryId (first variant)."""
    q = "&".join(f"ids={g}" for g in group_ids)
    r = client._get(f"/app/actionLibraryGroup/list?{q}")
    id_map = {}
    for d in (r.get("data") or []):
        lst = d.get("actionLibraryList") or []
        if lst:
            id_map[int(d["id"])] = lst[0]["id"]
    return id_map


def build_payload(client, name, exercises, device_type=1):
    """Construct the customTrainingTemplate POST body from a plan's exercises."""
    group_ids = list({int(ex["id"]) for ex in exercises})
    id_map = _resolve_variant_ids(client, group_ids)
    unilateral = {g: _is_unilateral(client, g) for g in group_ids}

    action_library_list = []
    total_capacity = 0.0

    for ex in exercises:
        group_id = int(ex["id"])
        variant_id = id_map.get(group_id)
        if not variant_id:
            raise ValueError(f"Could not resolve exercise id {group_id} ({ex.get('title','?')}). "
                             f"Run `speediance library --refresh` and check the id.")
        is_uni = unilateral.get(group_id, False)

        reps_list, weights_list, break_list = [], [], []
        mode_list, lr_list, level_list = [], [], []
        completion_list, completion_method_list, count_type_list = [], [], []
        set_capacity = 0.0

        for i, s in enumerate(ex["sets"]):
            reps = int(s.get("reps", 0))
            weight = float(s.get("weight", 0))
            mode = int(s.get("mode", 1))
            rest = int(s.get("rest", 60))

            reps_list.append(str(reps))
            break_list.append(str(rest))
            mode_list.append(str(mode))
            level_list.append("0")
            lr_list.append(("1" if i % 2 == 0 else "2") if is_uni else "0")
            completion_method_list.append("1")  # rep-based
            count_type_list.append("1")
            completion_list.append("1")

            api_weight = weight * KG_TO_API
            weights_list.append(f"{api_weight:.1f}")
            set_capacity += reps * api_weight

        total_capacity += set_capacity
        action_library_list.append({
            "groupId": group_id,
            "actionLibraryId": int(variant_id),
            "templatePresetId": -1,
            "setsAndReps": ",".join(reps_list),
            "breakTime": ",".join(break_list),
            "breakTime2": ",".join(break_list),
            "sportMode": ",".join(mode_list),
            "leftRight": ",".join(lr_list),
            "selectCompletionMethod": ",".join(completion_list),
            "completionMethod": ",".join(completion_method_list),
            "countType": ",".join(count_type_list),
            "weights": ",".join(weights_list),
            "counterweight2": "",
            "level": ",".join(level_list),
            "capacity": set_capacity,
        })

    return {
        "name": name,
        "actionLibraryList": action_library_list,
        "totalCapacity": total_capacity,
        "deviceType": device_type,
        "bgColor": 0,
    }


def create_template(client, name, exercises, device_type=1):
    payload = build_payload(client, name, exercises, device_type)
    r = client._post("/app/v2/customTrainingTemplate", payload)
    if r.get("code") != 0:
        raise RuntimeError(f"Create template failed: {r.get('message')} ({r.get('code')})")
    return r.get("data")


def load_plan(path):
    with open(path, encoding="utf-8") as f:
        plan = json.load(f)
    if "name" not in plan or "exercises" not in plan:
        raise ValueError("Plan JSON must have 'name' and 'exercises'.")
    return plan
