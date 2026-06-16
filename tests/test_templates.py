import os, sys, re
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from speediance import templates as tpl


class FakeClient:
    """Stubs the GETs templates needs and captures the POST. No network."""
    def __init__(self):
        self.posted = None
    def _get(self, path):
        if "actionLibraryGroup/list?" in path:
            ids = re.findall(r"ids=(\d+)", path)
            return {"code": 0, "data": [
                {"id": int(i), "actionLibraryList": [{"id": int(i) * 10}]} for i in ids]}
        if "actionLibraryGroup/" in path and "isDisplay=1" in path:
            return {"code": 0, "data": {"isLeftRight": 0}}
        return {"code": 0, "data": []}
    def _post(self, path, body):
        self.posted = (path, body)
        return {"code": 0, "data": {"id": 999, "name": body["name"]}}


def main():
    plan = {
        "name": "Test Push",
        "exercises": [
            {"id": 304, "sets": [
                {"reps": 15, "weight": 20, "mode": 1, "rest": 75},
                {"reps": 12, "weight": 22.5, "mode": 1, "rest": 75}]},
        ],
    }
    c = FakeClient()
    p = tpl.build_payload(c, plan["name"], plan["exercises"])
    a0 = p["actionLibraryList"][0]
    assert a0["groupId"] == 304 and a0["actionLibraryId"] == 3040
    assert a0["setsAndReps"] == "15,12"
    assert a0["weights"] == "44.0,49.5"            # kg * 2.2
    assert len(a0["weights"].split(",")) == len(a0["setsAndReps"].split(","))
    assert a0["leftRight"] == "0,0"
    tpl.create_template(c, plan["name"], plan["exercises"])
    assert c.posted[0] == "/app/v2/customTrainingTemplate"
    print("ALL TEMPLATE TESTS PASSED")


if __name__ == "__main__":
    main()
