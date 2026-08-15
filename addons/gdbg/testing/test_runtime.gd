extends Node

## Safe-idle addon autoload that owns playtest orchestration data.
##
## It parses and owns the existing --playtest, --playtest-seed,
## --playtest-output-dir, --playtest-frame-dir, --record and
## --playtest-background launch data, loads and validates the active playtest
## suite from a configurable pattern, and delegates every production lifecycle
## operation (setup/load/run/quit) to the DebugBridgeTestHost through
## Variant/Dictionary boundaries. It never loads a test-only main scene: the
## project main scene remains the production entry point and this node only
## resolves scripts and exposes state.

const RUNTIME_NODE_PATH := "/root/DebugBridgeTestRuntime"
const DEFAULT_SUITE_PATTERN := "res://tests/playtest/playtest_%s.gd"
const SUITE_PATTERN_SETTING := "gdbg/playtest/suite_pattern"
const TEST_HOST_SCRIPT := preload("res://addons/gdbg/testing/test_host.gd")

var _launch_args: Dictionary = {}
var _active_suite: DebugBridgePlaytestSuite = null
var _load_error: String = ""


## Resolve the registered runtime autoload node from the scene tree. Returns
## null when not registered, so callers fail closed.
static func instance() -> Variant:
	var tree := Engine.get_main_loop() as SceneTree
	if tree == null:
		return null
	var node := tree.root.get_node_or_null(RUNTIME_NODE_PATH)
	if node == null:
		return null
	return node


func _ready() -> void:
	process_mode = Node.PROCESS_MODE_ALWAYS
	_launch_args = _parse_launch_args()
	var playtest_name := String(_launch_args.get("playtest", "")).strip_edges()
	if playtest_name.is_empty():
		return
	DebugBridgePlaytestSuite.configure_process_seed(_launch_args)
	# Suite constructors may use the host project's services. Autoloads are
	# readied in registration order, so defer construction until every host
	# autoload (including the game lifecycle singleton) has completed _ready().
	call_deferred("_initialize_active_suite", playtest_name)


func _initialize_active_suite(playtest_name: String) -> void:
	if _active_suite != null or not _load_error.is_empty():
		return
	_active_suite = _load_suite(playtest_name)
	if _active_suite == null:
		_load_error = _load_error if not _load_error.is_empty() else "Failed to load playtest suite '%s'" % playtest_name
		push_error("[playtest] %s" % _load_error)
		production_quit(2)
		return
	_active_suite.apply_launch_args(_launch_args)


# ---------------------------------------------------------------------------
# Owned launch data
# ---------------------------------------------------------------------------

func launch_args() -> Dictionary:
	return _launch_args


func is_playtest_launch() -> bool:
	return not String(_launch_args.get("playtest", "")).is_empty()


func playtest_name() -> String:
	return String(_launch_args.get("playtest", ""))


func playtest_seed() -> int:
	return int(_launch_args.get("playtest_seed", 0))


func output_dir() -> String:
	return String(_launch_args.get("playtest_output_dir", ""))


func frame_dir() -> String:
	return String(_launch_args.get("playtest_frame_dir", ""))


func record_enabled() -> bool:
	return bool(_launch_args.get("record", false))


func is_background_launch() -> bool:
	return bool(_launch_args.get("playtest_background", false))


# ---------------------------------------------------------------------------
# Active suite
# ---------------------------------------------------------------------------

func active_suite() -> DebugBridgePlaytestSuite:
	# A caller can arrive before the deferred callback on unusual main scenes.
	# At that point all earlier autoloads are already ready, so initialize
	# synchronously and keep the public API deterministic.
	if _active_suite == null and _load_error.is_empty() and is_playtest_launch():
		_initialize_active_suite(playtest_name())
	return _active_suite


func load_error() -> String:
	return _load_error


# ---------------------------------------------------------------------------
# Production lifecycle delegation (setup/load/run/quit -> TestHost)
# ---------------------------------------------------------------------------

func production_setup() -> Variant:
	var host := _host()
	if host == null:
		return {"error": "DebugBridgeTestRuntime: DebugBridgeTestHost is not registered"}
	return host.playtest_boot(_launch_args)


func production_load_scenario(after_loaded_hook: Callable = Callable()) -> Variant:
	var host := _host()
	if host == null:
		return {"error": "DebugBridgeTestRuntime: DebugBridgeTestHost is not registered"}
	if _active_suite == null:
		return {"error": "DebugBridgeTestRuntime: no active playtest suite"}
	return host.playtest_load(
		_active_suite.scenario,
		after_loaded_hook,
		_active_suite.playtest_options()
	)


func production_world() -> Variant:
	var host := _host()
	if host == null:
		return null
	return host.playtest_world()


func production_run(world: Variant = null) -> int:
	var host := _host()
	if host == null:
		return 2
	if _active_suite == null:
		return 2
	if world == null:
		world = host.playtest_world()
	return int(await host.playtest_run(_active_suite, world))


func production_quit(exit_code: int = 0) -> void:
	var host := _host()
	if host == null:
		get_tree().quit(exit_code)
		return
	host.playtest_quit(exit_code)


# ---------------------------------------------------------------------------
# Launch parsing and suite loading
# ---------------------------------------------------------------------------

func _host() -> Variant:
	return TEST_HOST_SCRIPT.instance()


func _parse_launch_args() -> Dictionary:
	var result := {}
	var args := OS.get_cmdline_user_args()
	for i in range(args.size()):
		var arg: String = args[i]
		if arg == "--record":
			result["record"] = true
		elif arg.begins_with("--playtest="):
			result["playtest"] = arg.substr("--playtest=".length())
		elif arg == "--playtest" and i + 1 < args.size():
			result["playtest"] = args[i + 1]
		elif arg.begins_with("--playtest-seed="):
			result["playtest_seed"] = arg.substr("--playtest-seed=".length()).to_int()
		elif arg == "--playtest-seed" and i + 1 < args.size():
			result["playtest_seed"] = args[i + 1].to_int()
		elif arg.begins_with("--playtest-output-dir="):
			result["playtest_output_dir"] = arg.substr("--playtest-output-dir=".length())
		elif arg == "--playtest-output-dir" and i + 1 < args.size():
			result["playtest_output_dir"] = args[i + 1]
		elif arg.begins_with("--playtest-frame-dir="):
			result["playtest_frame_dir"] = arg.substr("--playtest-frame-dir=".length())
		elif arg == "--playtest-frame-dir" and i + 1 < args.size():
			result["playtest_frame_dir"] = args[i + 1]
		elif arg == "--playtest-background":
			result["playtest_background"] = true
	return result


func _load_suite(playtest_name: String) -> DebugBridgePlaytestSuite:
	var normalized := playtest_name.strip_edges().to_lower()
	if normalized.is_empty():
		_load_error = "Playtest name is empty"
		return null
	var pattern := String(ProjectSettings.get_setting(SUITE_PATTERN_SETTING, DEFAULT_SUITE_PATTERN))
	var path := pattern % normalized
	if not ResourceLoader.exists(path):
		_load_error = "Playtest script not found: %s" % path
		push_error("[playtest] %s" % _load_error)
		return null
	var script := load(path) as GDScript
	if script == null or not script.can_instantiate():
		_load_error = "Failed to load playtest script: %s" % path
		push_error("[playtest] %s" % _load_error)
		return null
	var instance = script.new()
	if not (instance is DebugBridgePlaytestSuite):
		_load_error = "Playtest script does not extend DebugBridgePlaytestSuite: %s" % path
		push_error("[playtest] %s" % _load_error)
		if instance is RefCounted:
			instance = null
		else:
			instance.free()
		return null
	var suite := instance as DebugBridgePlaytestSuite
	if suite.scenario == null or (suite.scenario is Dictionary and (suite.scenario as Dictionary).has("error")):
		_load_error = "Playtest script has no valid scenario: %s" % path
		push_error("[playtest] %s" % _load_error)
		return null
	return suite
