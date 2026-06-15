"""Config loading: JSON file + environment-variable overrides. Token cache."""
import os
import json

DEFAULT_WEEKS_DIR = r"C:\Users\gates\OneDrive\Documents\Claude\Projects\Workout\WEEKS"
CONFIG_PATH = os.environ.get("SPEEDIANCE_CONFIG", "config.json")
TOKEN_PATH = os.environ.get("SPEEDIANCE_TOKEN_CACHE", ".token.json")


def load_config():
    cfg = {
        "email": "",
        "password": "",
        "region": "Global",
        "unit": "lb",
        "weeks_dir": DEFAULT_WEEKS_DIR,
    }
    if os.path.exists(CONFIG_PATH):
        with open(CONFIG_PATH, "r", encoding="utf-8") as f:
            cfg.update(json.load(f))
    # env overrides
    cfg["email"] = os.environ.get("SPEEDIANCE_EMAIL", cfg["email"])
    cfg["password"] = os.environ.get("SPEEDIANCE_PASSWORD", cfg["password"])
    cfg["region"] = os.environ.get("SPEEDIANCE_REGION", cfg["region"])
    cfg["weeks_dir"] = os.environ.get("SPEEDIANCE_WEEKS_DIR", cfg["weeks_dir"])
    if not cfg["email"] or not cfg["password"]:
        raise SystemExit(
            "Missing credentials. Set them in config.json or via "
            "SPEEDIANCE_EMAIL / SPEEDIANCE_PASSWORD env vars."
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
