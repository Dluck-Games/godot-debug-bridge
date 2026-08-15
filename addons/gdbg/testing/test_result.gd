class_name DebugBridgeTestResult
extends RefCounted

## Lightweight assertion tool for integration tests.
## Collects pass/fail results and prints a CI-friendly report.

var _results: Array[Dictionary] = []


## Assert that a condition is true.
func assert_true(condition: bool, description: String = "") -> void:
	_results.append({
		"passed": condition,
		"description": description,
		"expected": "true",
		"actual": str(condition),
	})


## Assert that two values are equal.
func assert_equal(actual: Variant, expected: Variant, description: String = "") -> void:
	var is_equal: bool = (actual == expected)
	_results.append({
		"passed": is_equal,
		"description": description,
		"expected": str(expected),
		"actual": str(actual),
	})


## Whether all assertions passed.
func passed() -> bool:
	for r: Dictionary in _results:
		if not r["passed"]:
			return false
	return true


## 0 if all passed, 1 if any failed.
func exit_code() -> int:
	return 0 if passed() else 1


## Print a human- and CI-readable report to stdout.
func print_report() -> void:
	var pass_count: int = 0
	var total: int = _results.size()

	for i: int in range(total):
		var r: Dictionary = _results[i]
		var prefix: String = "[PASS]" if r["passed"] else "[FAIL]"
		var desc: String = r["description"] if r["description"] != "" else ("assertion %d" % (i + 1))
		if r["passed"]:
			print("%s %s" % [prefix, desc])
			pass_count += 1
		else:
			print("%s %s — expected: %s, got: %s" % [prefix, desc, r["expected"], r["actual"]])

	print("=== %d/%d passed ===" % [pass_count, total])
