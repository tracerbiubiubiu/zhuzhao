"""Golden expectations are fixed in fixtures, not computed by the model."""
import json
import unittest
from verify import HERE, run_case

DATA = json.loads((HERE / "fixtures.json").read_text())

class ContractCases(unittest.TestCase):
    pass

def case_test(case):
    def test(self):
        result = run_case(case, DATA["profile"])
        self.assertEqual([], result["failures"], result)
    return test

for case in DATA["cases"]:
    setattr(ContractCases, "test_" + case["id"], case_test(case))

if __name__ == "__main__":
    unittest.main(verbosity=2)
