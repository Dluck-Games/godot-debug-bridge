extends Node

## Game-agnostic IPC transport and capture orchestrator. Command execution and
## input injection delegate to /root/DebugBridgeHost; screenshots delegate to
## /root/ScreenshotManager. State and wire behavior live in their own helpers.

const POLL_INTERVAL := 0.1  # 100 ms polling interval

const MAX_SCREENSHOT_DELAY := 30.0
const MAX_SCREENSHOT_COUNT := 20
const MAX_SCREENSHOT_INTERVAL := 10.0
const MAX_SCREENSHOT_WIDTH := 3840
const MAX_SCREENSHOT_HEIGHT := 2160
const MAX_SCREENSHOT_SCALE := 4.0

const BRIDGE_HELP_TEXT := """

Bridge commands:
  input actions                    List injectable input actions
  input press <action>             Press and hold an input action
  input release <action>           Release an input action
  input tap <action>               Press for one frame, then release
  input hold <action> <seconds>    Hold an action for a duration
  screenshot --width/--height/--scale options are supported through gdbg debug
"""

const HOST_NODE_PATH := "/root/DebugBridgeHost"
const SCREENSHOT_NODE_PATH := "/root/ScreenshotManager"

var _poll_timer: float = 0.0
var _command_in_progress: bool = false
var _command_start_time: int = 0
# Max possible command duration: MAX_SCREENSHOT_DELAY + MAX_SCREENSHOT_COUNT * MAX_SCREENSHOT_INTERVAL + 10s buffer
const COMMAND_GUARD_TIMEOUT_MS := 240_000  # (30 + 20*10 + 10) * 1000 ms

const CAPTURE_EXPIRY_SECONDS := 60.0
const CAPTURE_CLEANUP_INTERVAL := 5.0

var _pending_captures: Dictionary = {}  # capture_id -> {paths, status, total, created_at, completed_at}
var _capture_cleanup_timer: float = 0.0


func _ready() -> void:
	_ensure_ipc_dir()
	_cleanup_stale_files()
	print("AIDebugBridge ready - waiting for AI commands")


func _cleanup_stale_files() -> void:
	if FileAccess.file_exists(DebugBridgeState.command_file()):
		DirAccess.remove_absolute(DebugBridgeState.command_file())
	if FileAccess.file_exists(DebugBridgeState.result_file()):
		DirAccess.remove_absolute(DebugBridgeState.result_file())
	if FileAccess.file_exists(DebugBridgeState.temp_script_file()):
		DirAccess.remove_absolute(DebugBridgeState.temp_script_file())


func _process(delta: float) -> void:
	# Cleanup runs unconditionally (even during command-in-progress)
	_capture_cleanup_timer += delta
	if _capture_cleanup_timer >= CAPTURE_CLEANUP_INTERVAL:
		_capture_cleanup_timer = 0.0
		_cleanup_expired_captures()

	if _command_in_progress:
		if _command_start_time > 0 and (_get_now_msec() - _command_start_time) > COMMAND_GUARD_TIMEOUT_MS:
			_report_stale_command_guard()
			_command_in_progress = false
			_command_start_time = 0
		else:
			return
	_poll_timer += delta
	if _poll_timer >= POLL_INTERVAL:
		_poll_timer = 0.0
		if FileAccess.file_exists(DebugBridgeState.command_file()):
			_execute_command_file()


func _ensure_ipc_dir() -> void:
	DebugBridgeState.ensure_dir(DebugBridgeState.ipc_dir())


func _report_stale_command_guard() -> void:
	push_warning("AIDebugBridge: command guard stuck for >%.0fs, force-resetting" % (float(COMMAND_GUARD_TIMEOUT_MS) / 1000.0))


# ============================================================
# IPC: File Protocol
# ============================================================

func _execute_command_file() -> void:
	var file := FileAccess.open(DebugBridgeState.command_file(), FileAccess.READ)
	if not file:
		return

	var content := file.get_as_text().strip_edges()
	file.close()
	DirAccess.remove_absolute(DebugBridgeState.command_file())

	if content.is_empty():
		return

	print("AIDebugBridge: executing command: ", content)

	var parsed := DebugBridgeProtocol.parse_command_content(content)
	match parsed["kind"]:
		DebugBridgeProtocol.KIND_JSON:
			# JSON protocol path
			_command_in_progress = true
			_command_start_time = _get_now_msec()
			await _execute_command_json(parsed["data"])
			_command_in_progress = false
			_command_start_time = 0
		DebugBridgeProtocol.KIND_JSON_INVALID:
			# Preserve current readable invalid JSON behavior
			_write_json_result({"result": null, "status": "error", "message": "Invalid JSON"})
		_:
			# Legacy plain-text path — forward directly to console
			_execute_plain_command(str(parsed["data"]))


func _execute_plain_command(content: String) -> void:
	var console_service := _get_console_service()
	if console_service == null:
		var error_text := _host_unavailable_message()
		print("AIDebugBridge: result: ", error_text)
		_write_result(error_text)
		return
	var result: String = console_service.execute(content)
	print("AIDebugBridge: result: ", result)
	_write_result(result)


func _write_result(content: String) -> void:
	var out := FileAccess.open(DebugBridgeState.result_file(), FileAccess.WRITE)
	if out:
		out.store_string(content)
		out.close()


func _write_json_result(data: Dictionary) -> void:
	_write_result(DebugBridgeProtocol.stringify_result(data))


func _host_unavailable_message() -> String:
	return "error: DebugBridgeHost not configured"


func _execute_command_json(cmd_dict: Dictionary) -> void:
	var protocol_version := int(cmd_dict.get("protocol", DebugBridgeProtocol.VERSION))
	if protocol_version != DebugBridgeProtocol.VERSION:
		_write_json_result({
			"result": null,
			"status": "error",
			"message": "Unsupported GDBG protocol version: %d" % protocol_version,
		})
		return
	var cmd: String = cmd_dict.get("cmd", "")
	if cmd.is_empty():
		_write_json_result({"result": null, "status": "error", "message": "Missing 'cmd' field"})
		return

	# Bridge-level commands: screenshot (async) and fetch (capture lifecycle)
	if cmd == "screenshot":
		await _handle_screenshot_json(_to_dictionary(cmd_dict.get("args", {})))
		return
	elif cmd == "fetch":
		_handle_fetch_json(_to_dictionary(cmd_dict.get("args", {})))
		return
	elif cmd == "input":
		await _handle_input_json(_to_dictionary(cmd_dict.get("args", {})))
		return

	# All other commands → forward to console
	var args_raw = cmd_dict.get("args", [])
	var console_cmd := cmd
	if args_raw is Array and not args_raw.is_empty():
		var str_args: Array[String] = []
		for a in args_raw:
			str_args.append(str(a))
		console_cmd += " " + " ".join(str_args)

	var console_service := _get_console_service()
	if console_service == null:
		_write_json_result({"result": null, "status": "error", "message": _host_unavailable_message()})
		return
	var result: String = console_service.execute(console_cmd)
	if cmd == "help":
		result = _augment_help_result(result, args_raw)

	# Check for piggyback screenshot
	var screenshot_opts = cmd_dict.get("screenshot", null)
	if screenshot_opts is Dictionary:
		var capture_id := _schedule_piggyback_screenshot(screenshot_opts)
		_write_json_result({"result": result, "capture_id": capture_id})
	else:
		_write_json_result({"result": result})


func _to_dictionary(value: Variant) -> Dictionary:
	if value is Dictionary:
		return value
	return {}


func _augment_help_result(result: String, args_raw: Variant) -> String:
	if not args_raw is Array or args_raw.is_empty():
		return result + BRIDGE_HELP_TEXT
	if str(args_raw[0]) == "input":
		return BRIDGE_HELP_TEXT.strip_edges()
	return result


# ============================================================
# Input Injection
# ============================================================

func _handle_input_json(args: Dictionary) -> void:
	var op: String = args.get("op", "")
	if op.is_empty():
		_write_json_result({"result": null, "status": "error", "message": "Missing input op"})
		return

	if op == "actions":
		_write_json_result({"result": _get_input_actions()})
		return

	var action: String = args.get("action", "")
	if action.is_empty():
		_write_json_result({"result": null, "status": "error", "message": "Missing input action"})
		return
	if not InputMap.has_action(action):
		_write_json_result({"result": null, "status": "error", "message": "Unknown input action: %s" % action})
		return

	match op:
		"press":
			_inject_action_press(action)
			_write_json_result({"result": "pressed %s" % action})
		"release":
			_inject_action_release(action)
			_write_json_result({"result": "released %s" % action})
		"tap":
			_inject_action_press(action)
			await get_tree().process_frame
			_inject_action_release(action)
			_write_json_result({"result": "tapped %s" % action})
		"hold":
			var duration: float = maxf(float(args.get("duration", 0.1)), 0.0)
			_inject_action_press(action)
			if duration > 0.0:
				await get_tree().create_timer(duration).timeout
			_inject_action_release(action)
			_write_json_result({"result": "held %s for %.2fs" % [action, duration]})
		_:
			_write_json_result({"result": null, "status": "error", "message": "Unknown input op: %s" % op})


func _inject_action_press(action: String) -> void:
	Input.action_press(action)
	var input_service: Variant = _get_input_service()
	if input_service != null:
		input_service.simulate_action_pressed(action)


func _inject_action_release(action: String) -> void:
	Input.action_release(action)
	var input_service: Variant = _get_input_service()
	if input_service != null:
		input_service.simulate_action_released(action)


func _get_input_actions() -> Array[String]:
	var actions: Array[String] = []
	for action_name in InputMap.get_actions():
		var action := str(action_name)
		if action.begins_with("ui_"):
			continue
		actions.append(action)
	actions.sort()
	return actions


# ============================================================
# Screenshot Orchestration
# ============================================================

func _generate_capture_id() -> String:
	var unix_ms := _get_unix_time_ms()
	return "cap_%d" % unix_ms


func _parse_screenshot_opts(opts: Dictionary) -> Dictionary:
	var parsed := {
		"delay": clampf(float(opts.get("delay", 0.0)), 0.0, MAX_SCREENSHOT_DELAY),
		"count": clampi(int(opts.get("count", 1)), 1, MAX_SCREENSHOT_COUNT),
		"interval": clampf(float(opts.get("interval", 1.0)), 0.1, MAX_SCREENSHOT_INTERVAL),
	}
	if opts.has("width"):
		parsed["width"] = clampi(int(opts.get("width", 0)), 1, MAX_SCREENSHOT_WIDTH)
	if opts.has("height"):
		parsed["height"] = clampi(int(opts.get("height", 0)), 1, MAX_SCREENSHOT_HEIGHT)
	if opts.has("scale"):
		parsed["scale"] = clampf(float(opts.get("scale", 0.0)), 0.1, MAX_SCREENSHOT_SCALE)
	if opts.has("dir"):
		var explicit_dir := str(opts["dir"]).strip_edges()
		if explicit_dir.is_absolute_path():
			parsed["dir"] = explicit_dir
	return parsed


func _capture_with_frame_sync(opts: Dictionary = {}) -> String:
	var screenshot_manager := get_node_or_null(SCREENSHOT_NODE_PATH)
	if screenshot_manager == null:
		return ""
	if DisplayServer.get_name() == "headless":
		return screenshot_manager.capture_now(opts)
	await RenderingServer.frame_post_draw
	return screenshot_manager.capture_now(opts)


func _handle_screenshot_json(args: Dictionary) -> void:
	var opts := _parse_screenshot_opts(args)
	var delay: float = opts["delay"]
	var count: int = opts["count"]
	var interval: float = opts["interval"]

	if delay > 0.0:
		await get_tree().create_timer(delay).timeout

	var paths: Array[String] = []
	for i in range(count):
		if i > 0:
			await get_tree().create_timer(interval).timeout
		var p := await _capture_with_frame_sync(opts)
		if not p.is_empty():
			paths.append(p)

	if paths.is_empty():
		_write_json_result({"result": null, "status": "error", "message": "Failed to capture screenshot"})
	elif paths.size() == 1:
		_write_json_result({"result": paths[0]})
	else:
		_write_json_result({"result": paths})


func _schedule_piggyback_screenshot(opts: Dictionary) -> String:
	var parsed := _parse_screenshot_opts(opts)
	var capture_id := _generate_capture_id()

	_pending_captures[capture_id] = {
		"paths": [] as Array[String],
		"status": "pending",
		"total": parsed["count"],
		"created_at": _get_now_msec(),
		"completed_at": 0,
	}

	# Fire-and-forget coroutine
	_run_piggyback_capture(capture_id, parsed)
	return capture_id


func _run_piggyback_capture(capture_id: String, opts: Dictionary) -> void:
	var delay: float = opts["delay"]
	var count: int = opts["count"]
	var interval: float = opts["interval"]

	if delay > 0.0:
		await get_tree().create_timer(delay).timeout

	for i in range(count):
		if i > 0:
			await get_tree().create_timer(interval).timeout
		var p := await _capture_with_frame_sync(opts)
		if not p.is_empty() and _pending_captures.has(capture_id):
			_pending_captures[capture_id]["paths"].append(p)

	if _pending_captures.has(capture_id):
		_pending_captures[capture_id]["status"] = "ready"
		_pending_captures[capture_id]["completed_at"] = _get_now_msec()


func _handle_fetch_json(args: Dictionary) -> void:
	var capture_id: String = args.get("capture_id", "")
	if capture_id.is_empty():
		_write_json_result({"result": null, "status": "error", "message": "Missing capture_id"})
		return

	if not _pending_captures.has(capture_id):
		_write_json_result({"result": null, "status": "error", "message": "Capture not found: %s" % capture_id})
		return

	var capture: Dictionary = _pending_captures[capture_id]
	if capture["status"] == "pending":
		var done: int = capture["paths"].size()
		var total: int = capture["total"]
		_write_json_result({"result": null, "status": "pending", "progress": "%d/%d" % [done, total]})
	else:
		_write_json_result({"result": capture["paths"], "status": "ready"})


func _cleanup_expired_captures() -> void:
	var now := _get_now_msec()
	# Max time a capture can stay pending before being considered stuck
	var pending_max_age_ms := int((MAX_SCREENSHOT_DELAY + MAX_SCREENSHOT_COUNT * MAX_SCREENSHOT_INTERVAL + CAPTURE_EXPIRY_SECONDS) * 1000)
	var expired: Array[String] = []
	for id in _pending_captures:
		var cap: Dictionary = _pending_captures[id]
		if cap["status"] == "ready" and cap["completed_at"] > 0:
			if (now - cap["completed_at"]) > CAPTURE_EXPIRY_SECONDS * 1000:
				expired.append(id)
		elif cap["status"] == "pending" and cap["created_at"] > 0:
			if (now - cap["created_at"]) > pending_max_age_ms:
				expired.append(id)
	for id in expired:
		_pending_captures.erase(id)


# ============================================================
# Host / Service accessors
# ============================================================
# Protected accessors kept so tests can inject doubles without touching /root nodes.

## Console host (project supplies execute()).
func _get_console_service() -> Variant:
	return get_node_or_null(HOST_NODE_PATH)


## Input host (project supplies simulate_action_pressed/released()).
func _get_input_service() -> Variant:
	return get_node_or_null(HOST_NODE_PATH)


func _get_now_msec() -> int:
	return Time.get_ticks_msec()


func _get_unix_time_ms() -> int:
	return int(Time.get_unix_time_from_system() * 1000)
