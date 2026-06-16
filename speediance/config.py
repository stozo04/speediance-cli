"""Config loading: JSON file + environment-variable overrides. Token cache.

Only `email` and `password` are required. `weeks_dir` is optional and used solely
by the `sync` command (which writes into Markdown week sheets); core commands
(login / workouts / session / library / push) never need it.

Device support: this CLI is built and tested for the **Gym Monster (v1)**
(`device_type = 1`). A **Gym Monster 2** exists and may use a different device
type and exercise ids - it is currently UNTESTED. Override with `device_type`
in config.json or the SPEEDIANCE_DEVICE_TYPE env var if you want to experiment.
"""
import os
import json

CONFIG_PATH = os.environ.get("SPEEDIANCE_CONFIG", "config.json")
TOKEN_PATH = os.environ.get("SPEEDIANCE_TOKEN_CACHE", ".token.json")


def load_config():
    cfg = {
        "email": "",
        "password": "",
        "region": "Global",     # Global | EU
        "unit": "lb",           # label only
        "device_type": 1,       # 1 = Gym Monster (v1). GM2 untested - see module note.
        "weeks_dir": "",        # optional; only `sync` uses it
    }
    if os.path.exists(CONFIG_PATH):
        with open(CONFIG_PATH, "r", encoding="utf-8") as f:
            cfg.update(json.load(f))
    cfg["email"] = os.environ.get("SPEEDIANCE_EMAIL", cfg["email"])
    cfg["password"] = os.environ.get("SPEEDIANCE_PASSWORD", cfg["password"])
    cfg["region"] = os.environ.get("SPEEDIANCE_REGION", cfg["region"])
    cfg["weeks_dir"] = os.environ.get("SPEEDIANCE_WEEKS_DIR", cfg["weeks_dir"])
    cfg["device_type"] = int(os.environ.get("SPEEDIANCE_DEVICE_TYPE", cfg["device_type"]))
    if not cfg["email"] or not cfg["password"]:
        raise SystemExit(
            "Missing credentials. Set email/password in config.json or via the "
            "SPEEDIANCE_EMAIL / SPEEDIANCE_PASSWORD environment variables."
        )
    return cfg


def load_token_cache():
    if os.path.exists(TOKEN_PATH):
        try:
            with open(TOKEN_PATH, "r", encoding="utf-8") as f:
                return json.load(f)
        except Exception:
            return {}
    return {}


def save_token_cache(token, user_id):
    with open(TOKEN_PATH, "w", encoding="utf-8") as f:
        json.dump({"token": token, "user_id": user_id}, f)
    try:
        import stat
        os.chmod(TOKEN_PATH, stat.S_IRUSR | stat.S_IWUSR)  # 0o600 — owner read/write only
    except (OSError, NotImplementedError):
        pass
