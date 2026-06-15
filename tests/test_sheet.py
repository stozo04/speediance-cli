import os, sys, shutil, tempfile
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from speediance.models import Workout, SetData
from speediance import sheet as sm
from datetime import date

DONE = "☑"


def make_workout():
    w = Workout(training_id=123, title="Push Day", workout_type="Strength",
                start_timestamp=0, duration_secs=2700, calories=310)
    w.completion_rate = 100.0

    def add(name, weight, reps):
        for i, r in enumerate(reps):
            w.sets.append(SetData(exercise_name=name, set_index=i + 1,
                                  finished_reps=r, target_reps=15, max_weight=weight))
    # Speediance-style names (differ from sheet wording on purpose)
    add("Chest Press", 45, [15, 15, 14])
    add("Incline Dumbbell Press", 30, [15, 13, 12])
    add("Seated Shoulder Press", 25, [15, 15, 15])
    add("Lateral Raise", 12, [18, 16, 15])
    add("Triceps Pushdown", 35, [15, 15, 13])
    add("Overhead Triceps Extension", 20, [15, 14])
    return w


def main():
    tmp = tempfile.mkdtemp()
    sheet = os.path.join(tmp, "Week-01.md")
    shutil.copy(os.path.join(os.path.dirname(__file__), "sample_week.md"), sheet)

    w = make_workout()
    res = sm.write_session(sheet, w, date(2026, 6, 15), unit="lb")

    out = open(sheet, encoding="utf-8").read()
    assert "45lb" in out, "weight not written"
    assert out.count(DONE) >= 6, "checkboxes not flipped"
    assert "Logged from Speediance" in out, "notes block missing"
    assert "______" not in out.split("## Notes")[0], "some weight cells left blank"
    glance = [l for l in out.splitlines() if l.strip().startswith("| **Mon 6/15**")][0]
    assert glance.strip().strip("|").split("|")[-1].strip() == DONE, "glance day not checked"
    print("ALL ASSERTIONS PASSED")
    print("matched:", res["matched"])


if __name__ == "__main__":
    main()
