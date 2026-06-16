"""Speediance cloud API client.

Auth flow + endpoints reverse-engineered from the official Android app, building
on the MIT-licensed UnofficialSpeedianceWorkoutManager (hbui3) and
speediance-influx (gavinmcfall) projects. Personal use, your own account/data.
"""
import time
import logging
from datetime import date, timedelta

import requests

from .models import Workout, SetData

logger = logging.getLogger("speediance")

BASE_URLS = {
    "Global": "https://api2.speediance.com/api",
    "EU": "https://euapi.speediance.com/api",
}


class AuthError(Exception):
    pass


class SpeedianceClient:
    def __init__(self, email, password, region="Global", token="", user_id=""):
        self.email = email
        self.password = password
        self.base = BASE_URLS.get(region, BASE_URLS["Global"])
        self.host = self.base.split("//")[1].split("/")[0]
        self.session = requests.Session()
        self.token = token or ""
        self.user_id = str(user_id or "")

    # ---- headers ------------------------------------------------------
    def _headers(self):
        h = {
            "Host": self.host,
            "User-Agent": "Dart/3.9 (dart:io)",
            "Content-Type": "application/json",
            "Timestamp": str(int(time.time() * 1000)),
            "Utc_offset": "+0000",
            "Timezone": "GMT",
            "Versioncode": "40304",
            "Accept-Language": "en",
            "App_type": "SOFTWARE",
            "Mobiledevices": '{"brand":"google","device":"emulator64","deviceType":"sdk_gphone64","os":"","os_version":"31","manufacturer":"Google"}',
        }
        if self.token:
            h["Token"] = self.token
        if self.user_id:
            h["App_user_id"] = self.user_id
        return h

    # ---- auth ---------------------------------------------------------
    def login(self):
        """Two-step email/password login. Returns (token, user_id)."""
        r = self.session.post(
            f"{self.base}/app/v2/login/verifyIdentity",
            json={"type": 2, "userIdentity": self.email},
            headers=self._headers(), timeout=20,
        ).json()
        if r.get("code") != 0:
            raise AuthError(f"verifyIdentity failed: {r.get('message')}")
        v = r.get("data", {})
        if not v.get("isExist"):
            raise AuthError("Account does not exist. Register in the Speediance app first.")
        if not v.get("hasPwd"):
            raise AuthError("Account has no password set. Set one in the Speediance app.")

        r = self.session.post(
            f"{self.base}/app/v2/login/byPass",
            json={"userIdentity": self.email, "password": self.password, "type": 2},
            headers=self._headers(), timeout=20,
        ).json()
        if r.get("code") != 0:
            raise AuthError(f"Login failed: {r.get('message')}")
        d = r["data"]
        self.token = d["token"]
        self.user_id = str(d["appUserId"])
        logger.info("Logged in as user %s", self.user_id)
        return self.token, self.user_id

    def _get(self, path, params=None):
        if not self.token:
            self.login()
        r = self.session.get(f"{self.base}{path}", headers=self._headers(),
                             params=params, timeout=20).json()
        if r.get("code") == 91:  # token expired
            logger.info("Token expired, re-authenticating")
            self.login()
            r = self.session.get(f"{self.base}{path}", headers=self._headers(),
                                 params=params, timeout=20).json()
        return r

    def _post(self, path, json_body):
        if not self.token:
            self.login()
        r = self.session.post(f"{self.base}{path}", headers=self._headers(),
                              json=json_body, timeout=30).json()
        if r.get("code") == 91:  # token expired
            self.login()
            r = self.session.post(f"{self.base}{path}", headers=self._headers(),
                                  json=json_body, timeout=30).json()
        return r

    # ---- data ---------------------------------------------------------
    def fetch_workouts(self, days=3):
        """Completed sessions over the last `days` days."""
        end = date.today() + timedelta(days=1)
        start = end - timedelta(days=days)
        r = self._get("/mobile/v2/report/userTrainingDataRecord",
                      params={"startDate": start.isoformat(), "endDate": end.isoformat()})
        if r.get("code") != 0:
            logger.warning("fetch_workouts failed: %s", r.get("message"))
            return []
        return [Workout.from_record(x) for x in (r.get("data") or [])]

    def fetch_detail(self, workout):
        """Populate per-set detail for a workout (mutates in place)."""
        info = self._get(f"/app/trainingInfo/cttTrainingInfo/{workout.training_id}")
        if info.get("code") == 0 and info.get("data"):
            workout.completion_rate = info["data"].get("completionRate", 0.0)

        r = self._get(f"/app/trainingInfo/cttTrainingInfoDetail/{workout.training_id}")
        if r.get("code") == 0 and isinstance(r.get("data"), list):
            for ex in r["data"]:
                name = ex.get("actionLibraryName", "")
                max_weight = ex.get("maxWeight", 0.0)
                for i, rep in enumerate(ex.get("finishedReps", [])):
                    workout.sets.append(SetData(
                        exercise_name=name,
                        set_index=i + 1,
                        finished_reps=rep.get("finishedCount", 0),
                        target_reps=rep.get("targetCount", 0),
                        max_weight=rep.get("weight", max_weight),
                        capacity=rep.get("capacity", 0.0),
                        max_heart_rate=rep.get("maxHeartRate", 0.0),
                        left_right=rep.get("leftRight", 0),
                    ))
        return workout
