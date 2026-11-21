class_name DebugBridgeRecordSession
extends Node

## Game-agnostic deterministic Movie Maker recording orchestrator.
##
## Activated automatically by DebugBridgeTestRuntime when `--gdbg-record-manifest`
## is passed on the Godot user command line. Loads an optional external director
## GDScript cleanly without touching project scenes, prevents macOS occlusion
## frame drops via offscreen RenderingServer.force_draw(), and logs exact frame
## boundaries (clip_begin, clip_end, mark, finish) to the JSONL manifest.

const ARG_MANIFEST := "--gdbg-record-manifest"
const ARG_SCRIPT := "--gdbg-record-script"
const ARG_FPS := "--gdbg-record-fps"
const ARG_SECONDS := "--gdbg-record-seconds"
const ARG_OFFSCREEN := "--gdbg-record-offscreen"

const DEFAULT_FPS := 30
const DEFAULT_SECONDS := 10.0

static var _current_session: DebugBridgeRecordSession = null

var _manifest_path: String = ""
var _script_path: String = ""
var _fps: int = DEFAULT_FPS
var _seconds: float = DEFAULT_SECONDS
var _offscreen_mode: String = "auto"
var _resolved_offscreen: String = "off"

var _manifest_file: FileAccess = null
var _frame_index: int = 0
var _post_draw_count: int = 0
var _last_seen_post_draw: int = 0
var _missing_draws: int = 0
var _active_clip_id: String = ""
var _finished: bool = false
var _post_draw_connected: bool = false
var _director: Node = null
var _caption_layer: CanvasLayer = null
var _caption_panel: PanelContainer = null
var _caption_label: Label = null


static func current() -> DebugBridgeRecordSession:
	return _current_session


static func has_launch_args() -> bool:
	var parsed := _parse_launch_args()
	return not String(parsed.get("manifest", "")).strip_edges().is_empty()


static func _parse_launch_args() -> Dictionary:
	var result := {}
	var args := OS.get_cmdline_user_args()
	for i in range(args.size()):
		var arg: String = args[i]
		if arg.begins_with(ARG_MANIFEST + "="):
			result["manifest"] = arg.substr(ARG_MANIFEST.length() + 1)
		elif arg == ARG_MANIFEST and i + 1 < args.size():
			result["manifest"] = args[i + 1]
		elif arg.begins_with(ARG_SCRIPT + "="):
			result["script"] = arg.substr(ARG_SCRIPT.length() + 1)
		elif arg == ARG_SCRIPT and i + 1 < args.size():
			result["script"] = args[i + 1]
		elif arg.begins_with(ARG_FPS + "="):
			result["fps"] = arg.substr(ARG_FPS.length() + 1).to_int()
		elif arg == ARG_FPS and i + 1 < args.size():
			result["fps"] = args[i + 1].to_int()
		elif arg.begins_with(ARG_SECONDS + "="):
			result["seconds"] = arg.substr(ARG_SECONDS.length() + 1).to_float()
		elif arg == ARG_SECONDS and i + 1 < args.size():
			result["seconds"] = args[i + 1].to_float()
		elif arg.begins_with(ARG_OFFSCREEN + "="):
			result["offscreen"] = arg.substr(ARG_OFFSCREEN.length() + 1).strip_edges().to_lower()
		elif arg == ARG_OFFSCREEN and i + 1 < args.size():
			result["offscreen"] = args[i + 1].strip_edges().to_lower()
	return result


func _enter_tree() -> void:
	_current_session = self
	process_mode = Node.PROCESS_MODE_ALWAYS
	process_priority = 2000000000


func _ready() -> void:
	var parsed := _parse_launch_args()
	_manifest_path = String(parsed.get("manifest", "")).strip_edges()
	_script_path = String(parsed.get("script", "")).strip_edges()
	_fps = maxi(1, int(parsed.get("fps", DEFAULT_FPS)))
	_seconds = maxf(0.1, float(parsed.get("seconds", DEFAULT_SECONDS)))
	_offscreen_mode = String(parsed.get("offscreen", "auto"))
	_resolved_offscreen = _resolve_offscreen_mode(_offscreen_mode)

	if not _manifest_path.is_empty():
		var base_dir := _manifest_path.get_base_dir()
		if not base_dir.is_empty():
			DirAccess.make_dir_recursive_absolute(base_dir)
		_manifest_file = FileAccess.open(_manifest_path, FileAccess.WRITE)
		if _manifest_file == null:
			push_error("DebugBridgeRecordSession: failed to open manifest %s" % _manifest_path)

	RenderingServer.frame_post_draw.connect(_on_frame_post_draw)
	_post_draw_connected = true
	_apply_offscreen_setup()
	_write_event({
		"type": "start",
		"fps": _fps,
		"offscreen": _resolved_offscreen,
		"process_frame": Engine.get_process_frames(),
	})
	call_deferred("_start_director")


func _exit_tree() -> void:
	if _post_draw_connected and RenderingServer.frame_post_draw.is_connected(_on_frame_post_draw):
		RenderingServer.frame_post_draw.disconnect(_on_frame_post_draw)
		_post_draw_connected = false
	if _manifest_file != null:
		_manifest_file.flush()
		_manifest_file.close()
		_manifest_file = null
	if _current_session == self:
		_current_session = null


func _process(_delta: float) -> void:
	if _finished:
		return
	if _resolved_offscreen == "force":
		var previous := _post_draw_count
		RenderingServer.force_draw(false, 1.0 / float(_fps))
		if _post_draw_count == previous:
			_missing_draws += 1
	elif _frame_index > 0:
		if _post_draw_count == _last_seen_post_draw:
			_missing_draws += 1
		_last_seen_post_draw = _post_draw_count
	_frame_index += 1


func _on_frame_post_draw() -> void:
	_post_draw_count += 1


func _resolve_offscreen_mode(mode: String) -> String:
	match mode:
		"force", "off":
			return mode
		"offscreen":
			return "offscreen"
		_:
			if OS.get_name() == "macOS":
				return "force"
			return "offscreen"


func _apply_offscreen_setup() -> void:
	if DisplayServer.get_name() == "headless":
		return
	var win := get_window()
	if _resolved_offscreen != "off":
		if win != null:
			win.unfocusable = true
		DisplayServer.window_set_flag(DisplayServer.WINDOW_FLAG_NO_FOCUS, true)
	if _resolved_offscreen == "force":
		DisplayServer.window_set_mode(DisplayServer.WINDOW_MODE_MINIMIZED)
		RenderingServer.render_loop_enabled = false
		if win != null:
			RenderingServer.viewport_set_update_mode(win.get_viewport_rid(), RenderingServer.VIEWPORT_UPDATE_ALWAYS)
	elif _resolved_offscreen == "offscreen":
		var mode := DisplayServer.window_get_mode()
		if mode == DisplayServer.WINDOW_MODE_FULLSCREEN or mode == DisplayServer.WINDOW_MODE_EXCLUSIVE_FULLSCREEN:
			DisplayServer.window_set_mode(DisplayServer.WINDOW_MODE_WINDOWED)
		DisplayServer.window_set_position(Vector2i(-12800, -12800))


func _reapply_window_mode_after_scene_ready() -> void:
	if DisplayServer.get_name() == "headless":
		return
	if _resolved_offscreen == "force":
		DisplayServer.window_set_mode(DisplayServer.WINDOW_MODE_MINIMIZED)
	elif _resolved_offscreen == "offscreen":
		DisplayServer.window_set_position(Vector2i(-12800, -12800))


func _start_director() -> void:
	_reapply_window_mode_after_scene_ready()
	if _script_path.is_empty():
		var total := maxi(1, ceili(_seconds * float(_fps)))
		while _frame_index < total:
			await wait_frames(1)
		finish(0)
		return

	if not FileAccess.file_exists(_script_path):
		_fail_director("Recording script not found: %s" % _script_path)
		return

	var source := FileAccess.get_file_as_string(_script_path)
	if source.is_empty():
		_fail_director("Recording script is empty: %s" % _script_path)
		return

	var script := GDScript.new()
	script.source_code = source
	var reload_err := script.reload()
	if reload_err != OK:
		_fail_director("Failed to compile recording script %s (err=%d)" % [_script_path, reload_err])
		return

	var instance: Variant = script.new()
	if not (instance is Node):
		if instance != null and not (instance is RefCounted):
			(instance as Object).free()
		_fail_director("Recording script must extend Node: %s" % _script_path)
		return

	_director = instance as Node
	_director.process_mode = Node.PROCESS_MODE_ALWAYS
	add_child(_director)

	if _director.has_method("run"):
		await _director.call("run", self)
		if not _finished:
			finish(0)


func _fail_director(message: String) -> void:
	_write_event({
		"type": "error",
		"message": message,
		"frame": _frame_index,
	})
	push_error("DebugBridgeRecordSession: %s" % message)
	finish(2)


# ============================================================
# Public Director API
# ============================================================

func frame_index() -> int:
	return _frame_index


func fps() -> int:
	return _fps


func missing_draws() -> int:
	return _missing_draws


## Mark the start of an edited output clip at the current frame index.
func begin_clip(id: String, title: String = "") -> void:
	var clean_id := id.strip_edges()
	if clean_id.is_empty():
		clean_id = "clip_%d" % _frame_index
	if not _active_clip_id.is_empty():
		end_clip(_active_clip_id)
	_active_clip_id = clean_id
	_write_event({
		"type": "clip_begin",
		"id": clean_id,
		"title": title,
		"frame": _frame_index,
		"process_frame": Engine.get_process_frames(),
		"post_draw": _post_draw_count,
		"missing_draws": _missing_draws,
	})


## Mark the end of the active output clip at the current frame index.
func end_clip(id: String = "") -> void:
	var clean_id := id.strip_edges()
	if clean_id.is_empty():
		clean_id = _active_clip_id
	if clean_id.is_empty():
		return
	if _active_clip_id == clean_id:
		_active_clip_id = ""
	_write_event({
		"type": "clip_end",
		"id": clean_id,
		"frame": _frame_index,
		"process_frame": Engine.get_process_frames(),
		"post_draw": _post_draw_count,
		"missing_draws": _missing_draws,
	})


## Record a keyframe marker for contact-sheet generation and manifest inspection.
func mark(marker_name: String) -> void:
	var clean := marker_name.strip_edges()
	if clean.is_empty():
		clean = "mark_%d" % _frame_index
	_write_event({
		"type": "mark",
		"name": clean,
		"clip_id": _active_clip_id,
		"frame": _frame_index,
		"process_frame": Engine.get_process_frames(),
		"post_draw": _post_draw_count,
		"missing_draws": _missing_draws,
	})


## Wait deterministically for `count` process frames.
func wait_frames(count: int) -> void:
	var tree := get_tree()
	if tree == null:
		return
	for _i in range(maxi(0, count)):
		await tree.process_frame


## Press an InputMap action via Input and optional DebugBridgeHost.
func press_action(action: String) -> void:
	if InputMap.has_action(action):
		Input.action_press(action)
	var host := get_node_or_null("/root/DebugBridgeHost")
	if host != null and host.has_method("simulate_action_pressed"):
		host.call("simulate_action_pressed", action)


## Release an InputMap action via Input and optional DebugBridgeHost.
func release_action(action: String) -> void:
	if InputMap.has_action(action):
		Input.action_release(action)
	var host := get_node_or_null("/root/DebugBridgeHost")
	if host != null and host.has_method("simulate_action_released"):
		host.call("simulate_action_released", action)


## Tap an InputMap action for `hold_frames` frames, then release.
func tap_action(action: String, hold_frames: int = 2) -> void:
	await hold_action(action, hold_frames)


## Hold an InputMap action for `frames` frames, then release.
func hold_action(action: String, frames: int) -> void:
	press_action(action)
	await wait_frames(maxi(1, frames))
	release_action(action)


## Push a synthetic InputEventKey press/release to the viewport.
func press_key(keycode: Key, hold_frames: int = 2) -> void:
	var vp := get_viewport()
	if vp == null:
		return
	var ev := InputEventKey.new()
	ev.keycode = keycode
	ev.physical_keycode = keycode
	ev.pressed = true
	vp.push_input(ev)
	await wait_frames(maxi(1, hold_frames))
	var rel := InputEventKey.new()
	rel.keycode = keycode
	rel.physical_keycode = keycode
	rel.pressed = false
	vp.push_input(rel)


## Execute a project console command through /root/DebugBridgeHost if present.
func console(command: String) -> String:
	var host := get_node_or_null("/root/DebugBridgeHost")
	if host == null or not host.has_method("execute"):
		return "error: DebugBridgeHost not configured"
	return str(host.call("execute", command))


## Display a native bottom caption overlay inside the rendered viewport.
func set_caption(text: String) -> void:
	if text.strip_edges().is_empty():
		clear_caption()
		return
	_ensure_caption_overlay()
	_caption_label.text = text
	_caption_panel.visible = true


## Hide the native caption overlay.
func clear_caption() -> void:
	if _caption_panel != null and is_instance_valid(_caption_panel):
		_caption_panel.visible = false


## Flush manifest events and quit the engine cleanly.
func finish(exit_code: int = 0) -> void:
	if _finished:
		return
	_finished = true
	if not _active_clip_id.is_empty():
		end_clip(_active_clip_id)
	_write_event({
		"type": "finish",
		"frame": _frame_index,
		"process_frame": Engine.get_process_frames(),
		"post_draw": _post_draw_count,
		"missing_draws": _missing_draws,
		"exit_code": exit_code,
	})
	if _manifest_file != null:
		_manifest_file.flush()
		_manifest_file.close()
		_manifest_file = null
	var tree := get_tree()
	if tree != null:
		tree.quit(exit_code)


func _ensure_caption_overlay() -> void:
	if _caption_layer != null and is_instance_valid(_caption_layer):
		return
	_caption_layer = CanvasLayer.new()
	_caption_layer.layer = 128

	var margin := MarginContainer.new()
	margin.set_anchors_and_offsets_preset(Control.PRESET_BOTTOM_WIDE)
	margin.mouse_filter = Control.MOUSE_FILTER_IGNORE
	margin.add_theme_constant_override("margin_left", 32)
	margin.add_theme_constant_override("margin_right", 32)
	margin.add_theme_constant_override("margin_bottom", 24)

	_caption_panel = PanelContainer.new()
	_caption_panel.mouse_filter = Control.MOUSE_FILTER_IGNORE
	_caption_panel.size_flags_horizontal = Control.SIZE_SHRINK_CENTER
	var style := StyleBoxFlat.new()
	style.bg_color = Color(0.06, 0.07, 0.11, 0.84)
	style.border_color = Color(0.45, 0.52, 0.65, 0.8)
	style.set_border_width_all(1)
	style.set_corner_radius_all(6)
	style.set_content_margin_all(10)
	style.content_margin_left = 18
	style.content_margin_right = 18
	_caption_panel.add_theme_stylebox_override("panel", style)

	_caption_label = Label.new()
	_caption_label.horizontal_alignment = HORIZONTAL_ALIGNMENT_CENTER
	_caption_label.add_theme_font_size_override("font_size", 22)
	_caption_label.add_theme_color_override("font_color", Color(0.96, 0.96, 0.94, 1.0))
	_caption_panel.add_child(_caption_label)

	margin.add_child(_caption_panel)
	_caption_layer.add_child(margin)
	add_child(_caption_layer)


func _write_event(payload: Dictionary) -> void:
	if _manifest_file == null:
		return
	_manifest_file.store_line(JSON.stringify(payload))
	_manifest_file.flush()
