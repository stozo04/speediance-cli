"""Dataclasses for Speediance workout data (subset we need for syncing)."""
from dataclasses import dataclass, field
from datetime import datetime


@dataclass
class SetData:
    exercise_name: str
    set_index: int
    finished_reps: int = 0
    target_reps: int = 0
    max_weight: float = 0.0
    capacity: float = 0.0          # volume (weight*reps) reported per set
    max_heart_rate: float = 0.0
    left_right: int = 0            # 0=both, 1=left, 2=right


@dataclass
class Workout:
    training_id: int
    title: str
    workout_type: str
    start_timestamp: int = 0
    end_timestamp: int = 0
    duration_secs: int = 0
    calories: int = 0
    volume_kg: float = 0.0
    completion_rate: float = 0.0
    sets: list = field(default_factory=list)

    @classmethod
    def from_record(cls, r: dict) -> "Workout":
        return cls(
            training_id=r.get("trainingId", 0) or r.get("id", 0),
            title=r.get("title", "") or "Workout",
            workout_type=r.get("courseTypeStr", "") or r.get("courseCategoryName", ""),
            start_timestamp=r.get("startTimestamp", 0),
            end_timestamp=r.get("endTimestamp", 0),
            duration_secs=r.get("trainingTime", 0),
            calories=r.get("calorie", 0),
            volume_kg=r.get("totalCapacity", 0.0),
        )

    @property
    def date(self):
        ts = self.start_timestamp or self.end_timestamp
        if not ts:
            return None
        # Speediance sends seconds; some endpoints may send milliseconds.
        # Normalize: anything above 1e12 is milliseconds.
        ts = float(ts)
        if ts > 1e12:
            ts /= 1000.0
        return datetime.fromtimestamp(ts).date()

    def exercises(self):
        """Group sets by exercise name, preserving first-seen order."""
        out = {}
        for s in self.sets:
            out.setdefault(s.exercise_name, []).append(s)
        return out
