class_name DebugBridgePlaytestSuite
extends "res://addons/gdbg/testing/automation_test_suite.gd"

## Generic playtest orchestration: deterministic seed, checkpoints, timeout,
## reports and optional frame recording. Scenario construction and every game
## lifecycle operation remain behind DebugBridgeTestHostBase.

enum Status {
	RUNNING,
	PASSED,
	FAILED,
	ERROR,
}

const DEFAULT_TIMEOUT_SECONDS: float = 300.0
const DEFAULT_RANDOM_SEED: int = 0x47444247
const RECORDING_FPS: int = 4
const REPORT_FILE_NAME: String = "report.txt"
const TIMELINE_FILE_NAME: String = "timeline.log"

var _checkpoints: Array[Dictionary] = []
var _current_checkpoint_index: int = 0
var _output_dir: String = ""
var _frame_dir: String = ""
var _record_enabled: bool = false
var _recorder: DebugBridgePlaytestRecorder = null
var _start_msec: int = 0
var _status: int = Status.RUNNING
var _status_detail: String = ""
var _configured_random_seed: int = DEFAULT_RANDOM_SEED


func _init(p_scenario: Variant = null) -> void:
	super(p_scenario)
	after_loaded_hook = Callable(self, "after_scenario_loaded")


func suite_name() -> String:
	push_error("DebugBridgePlaytestSuite.suite_name() must be overridden")
	return ""


static func deterministic_seed_for_suite(name: String) -> int:
	return DEFAULT_RANDOM_SEED ^ name.strip_edges().to_lower().hash()


static func configure_process_seed(args: Dictionary) -> int:
	var name := String(args.get("playtest", "")).strip_edges().to_lower()
	if name.is_empty():
		return 0
	var configured_seed := int(args.get("playtest_seed", deterministic_seed_for_suite(name)))
	seed(configured_seed)
	return configured_seed


func configured_random_seed() -> int:
	return _configured_random_seed


func timeout_seconds() -> float:
	return DEFAULT_TIMEOUT_SECONDS


func setup_checkpoints() -> void:
	pass


func check_next_checkpoint(_world: Variant) -> bool:
	return false


func apply_launch_args(args: Dictionary) -> void:
	var seed_args := args.duplicate()
	if String(seed_args.get("playtest", "")).is_empty():
		seed_args["playtest"] = suite_name()
	_configured_random_seed = configure_process_seed(seed_args)
	_record_enabled = bool(args.get("record", false))
	_output_dir = String(args.get("playtest_output_dir", ""))
	_frame_dir = String(args.get("playtest_frame_dir", ""))


func register_checkpoint(name: String) -> void:
	if name.is_empty():
		return
	_checkpoints.append({
		"name": name,
		"passed": false,
		"time": -1.0,
	})


func pass_checkpoint(name: String) -> void:
	for i in range(_checkpoints.size()):
		var checkpoint: Dictionary = _checkpoints[i]
		if String(checkpoint.get("name", "")) != name:
			continue
		if bool(checkpoint.get("passed", false)):
			return
		checkpoint["passed"] = true
		checkpoint["time"] = _elapsed_seconds()
		_checkpoints[i] = checkpoint
		_playtest_log("%.1fs  PASS  %s" % [float(checkpoint["time"]), name])
		if i == _current_checkpoint_index:
			_current_checkpoint_index += 1
		return


func is_checkpoint_passed(name: String) -> bool:
	for checkpoint: Dictionary in _checkpoints:
		if String(checkpoint.get("name", "")) == name:
			return bool(checkpoint.get("passed", false))
	return false


func all_checkpoints_passed() -> bool:
	return not _checkpoints.is_empty() and _current_checkpoint_index >= _checkpoints.size()


func current_checkpoint_name() -> String:
	if _current_checkpoint_index < 0 or _current_checkpoint_index >= _checkpoints.size():
		return ""
	return String(_checkpoints[_current_checkpoint_index].get("name", ""))


func after_scenario_loaded(world: Variant) -> void:
	_start_msec = Time.get_ticks_msec()
	_checkpoints.clear()
	_current_checkpoint_index = 0
	setup_checkpoints()
	_prepare_output_dirs()
	if _record_enabled:
		_start_recording(world)


## Default data-driven checkpoint loop. A project suite may instead define its
## own typed `test_run(world)` method; the host adapter chooses it when present.
func run_default(world: Variant) -> Variant:
	if _start_msec <= 0:
		_start_msec = Time.get_ticks_msec()
	if _checkpoints.is_empty():
		_mark_error("No checkpoints registered")
		_finish(world)
		return _to_test_result()

	_playtest_log("start seed=%d timeout=%.0fs" % [_configured_random_seed, timeout_seconds()])
	var waiting_for := ""
	while not all_checkpoints_passed():
		if _elapsed_seconds() > timeout_seconds():
			_mark_error("Timed out waiting for checkpoint: %s" % current_checkpoint_name())
			break
		var checkpoint_name := current_checkpoint_name()
		if checkpoint_name != waiting_for and not checkpoint_name.is_empty():
			_playtest_log("%.1fs  wait  %s" % [_elapsed_seconds(), checkpoint_name])
			waiting_for = checkpoint_name
		if check_next_checkpoint(world):
			if not checkpoint_name.is_empty():
				pass_checkpoint(checkpoint_name)
		await _wait_frames(world, 1)

	if all_checkpoints_passed():
		_status = Status.PASSED
		_status_detail = ""
	elif _status == Status.RUNNING:
		_status = Status.FAILED
		_status_detail = "Checkpoint failed: %s" % current_checkpoint_name()
		_playtest_log("%.1fs  FAIL  %s" % [_elapsed_seconds(), _status_detail])

	_finish(world)
	return _to_test_result()


func exit_code() -> int:
	match _status:
		Status.PASSED:
			return 0
		Status.ERROR:
			return 2
	return 1


func _mark_error(message: String) -> void:
	if _status == Status.ERROR:
		return
	_status = Status.ERROR
	_status_detail = message
	_playtest_log("%.1fs  ERROR %s" % [_elapsed_seconds(), message])


func _finish(_world: Variant) -> void:
	var passed_count := 0
	for checkpoint: Dictionary in _checkpoints:
		if bool(checkpoint.get("passed", false)):
			passed_count += 1
	_playtest_log("%.1fs  %s  %d/%d" % [_elapsed_seconds(), _status_text(), passed_count, _checkpoints.size()])
	_stop_recording()
	_write_reports()
	_print_report()


func _to_test_result() -> DebugBridgeTestResult:
	var result := DebugBridgeTestResult.new()
	result.assert_true(_status == Status.PASSED, "Playtest %s passed" % suite_name())
	return result


func _elapsed_seconds() -> float:
	if _start_msec <= 0:
		return 0.0
	return float(Time.get_ticks_msec() - _start_msec) / 1000.0


func _prepare_output_dirs() -> void:
	_make_dir(_report_dir())
	_make_dir(_user_report_dir())
	if _record_enabled:
		_make_dir(_record_frame_dir())


func _start_recording(world: Variant) -> void:
	if world == null:
		_mark_error("Cannot record without a world")
		return
	var tree: SceneTree = null
	if world is Node:
		tree = (world as Node).get_tree()
	if tree == null:
		tree = Engine.get_main_loop() as SceneTree
	if tree == null or tree.root == null:
		_mark_error("Cannot record without a SceneTree root")
		return
	_recorder = DebugBridgePlaytestRecorder.new()
	_recorder.configure(_record_frame_dir(), RECORDING_FPS, tree.root)
	# Attach to the SceneTree root (not the gameplay world) with
	# PROCESS_MODE_ALWAYS so _process fires every frame for the whole play test,
	# even during day-rollover pauses or world teardown. The recorder must not
	# depend on a gameplay world subtree, which can be paused and is gameplay-tied.
	tree.root.add_child(_recorder)
	_recorder.start()


func _stop_recording() -> void:
	if _recorder == null:
		return
	_recorder.stop()
	if is_instance_valid(_recorder):
		# Detach from the root immediately so no stray frame fires after stop and
		# the node cannot leak into any subsequent (formal) run.
		if _recorder.get_parent() != null:
			_recorder.get_parent().remove_child(_recorder)
		_recorder.queue_free()
	_recorder = null


func _write_reports() -> void:
	var report := _build_report()
	_write_text(_report_path(), report)
	var user_path := _user_report_path()
	if user_path != _report_path():
		_write_text(user_path, report)


func _print_report() -> void:
	for line in _build_report().split("\n"):
		print(line)


func _playtest_log(message: String) -> void:
	var line := "[playtest:%s] %s" % [suite_name(), message]
	printerr(line)
	_append_timeline(line)


func _append_timeline(line: String) -> void:
	if _output_dir.is_empty():
		return
	var path := _globalize(_output_dir.path_join(TIMELINE_FILE_NAME))
	_make_dir(path.get_base_dir())
	var flags := FileAccess.WRITE
	if FileAccess.file_exists(path):
		flags = FileAccess.READ_WRITE
	var file := FileAccess.open(path, flags)
	if file == null:
		return
	if flags == FileAccess.READ_WRITE:
		file.seek_end()
	file.store_line(line)
	file.flush()
	file.close()


func _build_report() -> String:
	var lines: Array[String] = []
	lines.append("=== PLAYTEST: %s ===" % suite_name())
	lines.append("Status: %s" % _status_text())
	lines.append("Seed: %d" % _configured_random_seed)
	if not _status_detail.is_empty():
		lines.append("Detail: %s" % _status_detail)
	lines.append("")
	lines.append("Checkpoints:")
	var passed_count := 0
	for checkpoint: Dictionary in _checkpoints:
		var passed := bool(checkpoint.get("passed", false))
		if passed:
			passed_count += 1
		var mark := "PASS" if passed else "FAIL"
		var checkpoint_time := float(checkpoint.get("time", -1.0))
		var time_text := "%.1fs" % checkpoint_time if checkpoint_time >= 0.0 else "--"
		lines.append("  [%s]  %-32s (%s)" % [mark, String(checkpoint.get("name", "")), time_text])
	lines.append("")
	lines.append("Total: %d/%d passed in %.1fs" % [passed_count, _checkpoints.size(), _elapsed_seconds()])
	return "\n".join(lines)


func _status_text() -> String:
	match _status:
		Status.PASSED:
			return "PASSED"
		Status.FAILED:
			return "FAILED"
		Status.ERROR:
			return "ERROR"
	return "RUNNING"


func _report_dir() -> String:
	if not _output_dir.is_empty():
		return _output_dir
	return _user_report_dir()


func _user_report_dir() -> String:
	return "user://playtest_results/%s" % suite_name()


func _report_path() -> String:
	return _report_dir().path_join(REPORT_FILE_NAME)


func _user_report_path() -> String:
	return _user_report_dir().path_join(REPORT_FILE_NAME)


func _record_frame_dir() -> String:
	if not _frame_dir.is_empty():
		return _frame_dir
	return "user://playtest_frames/%s" % suite_name()


func _make_dir(path: String) -> void:
	var absolute_path := _globalize(path)
	DirAccess.make_dir_recursive_absolute(absolute_path)


func _write_text(path: String, text: String) -> void:
	_make_dir(path.get_base_dir())
	var file := FileAccess.open(path, FileAccess.WRITE)
	if file == null:
		push_error("DebugBridgePlaytestSuite: failed to write %s" % path)
		return
	file.store_string(text)
	file.close()


func _globalize(path: String) -> String:
	if path.begins_with("user://") or path.begins_with("res://"):
		return ProjectSettings.globalize_path(path)
	return path
