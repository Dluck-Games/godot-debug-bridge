extends Node

## On-demand, game-agnostic viewport capture service. Installed as the
## ScreenshotManager autoload; output paths are resolved by DebugBridgeState.

const SCREENSHOT_PREFIX := "ai_screenshot_"
const SCREENSHOT_EXTENSION := ".png"
const MAX_CAPTURE_WIDTH := 3840
const MAX_CAPTURE_HEIGHT := 2160
const MAX_HISTORY := 100

@export var enabled: bool = true

var _viewport: Viewport = null
var _latest_screenshot_path: String = ""


func _ready() -> void:
	_ensure_runtime_dirs()
	_viewport = get_viewport()
	_restore_latest_screenshot_path()


func _ensure_runtime_dirs() -> void:
	DebugBridgeState.ensure_dir(DebugBridgeState.screenshots_dir())


func _resolve_output_dir(options: Dictionary) -> String:
	var explicit_dir := str(options.get("dir", "")).strip_edges()
	if not explicit_dir.is_empty() and explicit_dir.is_absolute_path():
		return explicit_dir
	return DebugBridgeState.screenshots_dir()


func _restore_latest_screenshot_path() -> void:
	var screenshot_files := _list_screenshot_files()
	if screenshot_files.is_empty():
		_latest_screenshot_path = ""
		return

	screenshot_files.sort()
	var latest_file: String = screenshot_files[screenshot_files.size() - 1]
	_latest_screenshot_path = DebugBridgeState.screenshots_dir().path_join(latest_file)


func _capture_and_save(options: Dictionary = {}) -> String:
	if _viewport == null:
		push_warning("ScreenshotManager: viewport not ready")
		return ""

	var texture := _viewport.get_texture()
	if texture == null:
		push_warning("ScreenshotManager: viewport texture is null")
		return ""

	var image := texture.get_image()
	if image == null:
		push_warning("ScreenshotManager: image is null")
		return ""

	var source_size := Vector2i(image.get_width(), image.get_height())
	var capture_size := _resolve_capture_size(source_size, options)
	if capture_size != source_size:
		image.resize(capture_size.x, capture_size.y, Image.INTERPOLATE_NEAREST)

	var screenshot_path := _build_screenshot_path(options)
	DebugBridgeState.ensure_dir(screenshot_path.get_base_dir())
	var err := image.save_png(screenshot_path)
	if err != OK:
		push_warning("ScreenshotManager: failed to save screenshot, error: %s" % err)
		return ""

	_latest_screenshot_path = screenshot_path
	_prune_old_screenshots()
	return ProjectSettings.globalize_path(screenshot_path)


func _resolve_capture_size(source_size: Vector2i, options: Dictionary) -> Vector2i:
	if source_size.x <= 0 or source_size.y <= 0:
		return source_size

	var scale := float(options.get("scale", 0.0))
	if scale > 0.0:
		return _clamp_capture_size(Vector2i(
			maxi(1, roundi(float(source_size.x) * scale)),
			maxi(1, roundi(float(source_size.y) * scale))
		))

	var target_width := int(options.get("width", 0))
	var target_height := int(options.get("height", 0))
	if target_width <= 0 and target_height <= 0:
		return _clamp_capture_size(source_size)

	if target_width <= 0:
		target_width = roundi(float(source_size.x) * float(target_height) / float(source_size.y))
	elif target_height <= 0:
		target_height = roundi(float(source_size.y) * float(target_width) / float(source_size.x))

	return _clamp_capture_size(Vector2i(maxi(1, target_width), maxi(1, target_height)))


func _clamp_capture_size(size: Vector2i) -> Vector2i:
	var width := maxi(1, size.x)
	var height := maxi(1, size.y)
	var ratio := minf(
		1.0,
		minf(
			float(MAX_CAPTURE_WIDTH) / float(width),
			float(MAX_CAPTURE_HEIGHT) / float(height)
		)
	)
	if ratio >= 1.0:
		return Vector2i(width, height)
	return Vector2i(maxi(1, roundi(float(width) * ratio)), maxi(1, roundi(float(height) * ratio)))


func _build_screenshot_path(options: Dictionary = {}) -> String:
	var datetime := Time.get_datetime_dict_from_system()
	var timestamp := "%04d%02d%02d_%02d%02d%02d_%03d_%d" % [
		datetime.year,
		datetime.month,
		datetime.day,
		datetime.hour,
		datetime.minute,
		datetime.second,
		Time.get_ticks_msec() % 1000,
		Time.get_ticks_usec() % 1000
	]
	var file_name := "%s%s%s" % [SCREENSHOT_PREFIX, timestamp, SCREENSHOT_EXTENSION]
	return _resolve_output_dir(options).path_join(file_name)


func _prune_old_screenshots() -> void:
	var screenshot_files := _list_screenshot_files()
	if screenshot_files.size() <= MAX_HISTORY:
		return

	screenshot_files.sort()
	var delete_count := screenshot_files.size() - MAX_HISTORY
	for index in range(delete_count):
		var old_path := DebugBridgeState.screenshots_dir().path_join(screenshot_files[index])
		DirAccess.remove_absolute(old_path)


func _list_screenshot_files() -> Array[String]:
	var files: Array[String] = []
	var dir := DirAccess.open(DebugBridgeState.screenshots_dir())
	if not dir:
		return files

	dir.list_dir_begin()
	var file_name := dir.get_next()
	while file_name != "":
		if not dir.current_is_dir() and file_name.begins_with(SCREENSHOT_PREFIX) and file_name.ends_with(SCREENSHOT_EXTENSION):
			files.append(file_name)
		file_name = dir.get_next()
	dir.list_dir_end()

	return files


## Capture one screenshot immediately.
func capture_now(options: Dictionary = {}) -> String:
	if _viewport == null:
		_viewport = get_viewport()
	return _capture_and_save(options)


## Return the absolute path of the latest screenshot.
func get_screenshot_path() -> String:
	if _latest_screenshot_path.is_empty():
		return ""
	return ProjectSettings.globalize_path(_latest_screenshot_path)
