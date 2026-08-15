class_name DebugBridgePlaytestRecorder
extends Node

## Continuous playtest frame recorder, extracted from the game-owned
## AutomationPlayTestSuite so the addon can own recording without importing any
## game types.
##
## The recorder is fully generic: it captures the root viewport (passed in
## configure, falling back to its own get_viewport() when attached under the
## SceneTree root) and never references project world types.

# Canonical continuous-recording frame size. Matches the named checkpoint
# screenshot artifacts so repeated gameplay runs produce compact, consistent
# frames regardless of the actual game viewport resolution.
const FRAME_SIZE := Vector2i(960, 540)

var frame_dir: String = ""
var fps: int = 4
var _viewport: Viewport = null
var _running: bool = false
var _frame_index: int = 0
var _accumulator: float = 0.0


## `p_viewport` may be the SceneTree root viewport; when null the recorder falls
## back to get_viewport(), which is the root viewport when this node lives under
## the SceneTree root.
func configure(p_frame_dir: String, p_fps: int, p_viewport: Viewport = null) -> void:
	frame_dir = p_frame_dir
	fps = maxi(p_fps, 1)
	_viewport = p_viewport
	DirAccess.make_dir_recursive_absolute(_globalize_local(frame_dir))


# PROCESS_MODE_ALWAYS is set on enter-tree (before any processing) so the
# recorder samples every frame regardless of SceneTree.paused (day rollover).
func _ready() -> void:
	process_mode = Node.PROCESS_MODE_ALWAYS


func start() -> void:
	_running = true
	_accumulator = 0.0
	process_mode = Node.PROCESS_MODE_ALWAYS
	# Explicitly request processing rather than relying on the default; stop()
	# disables it, so each run has a clean start/stop pair.
	set_process(true)
	_capture_frame()


func stop() -> void:
	var was_running := _running
	_running = false
	if was_running:
		_capture_frame()
	set_process(false)


func _process(delta: float) -> void:
	if not _running:
		return
	_accumulator += delta
	var interval := 1.0 / float(fps)
	# Preserve elapsed sampling time instead of resetting the accumulator.
	# PNG persistence and named checkpoint captures can make one process frame
	# span several sample intervals; duplicate the latest rendered image for
	# those intervals so the encoded video keeps wall-clock duration.
	while _accumulator >= interval:
		_accumulator -= interval
		_capture_frame()


func _capture_frame() -> void:
	# Capture the root viewport: either the one supplied to configure(), or the
	# viewport of this node (the main window) when attached under the SceneTree
	# root, matching the checkpoint screenshot capture path.
	var viewport := _viewport if _viewport != null else get_viewport()
	if viewport == null:
		return
	var texture := viewport.get_texture()
	if texture == null:
		return
	var image := texture.get_image()
	if image == null:
		return
	# Normalize every continuous frame to FRAME_SIZE before persistence so
	# the saved PNGs are exactly 960x540, matching the acceptance-video
	# contract and the checkpoint screenshot artifacts. INTERPOLATE_NEAREST
	# mirrors the screenshot_manager capture path.
	var source_size := Vector2i(image.get_width(), image.get_height())
	if source_size != FRAME_SIZE:
		image.resize(FRAME_SIZE.x, FRAME_SIZE.y, Image.INTERPOLATE_NEAREST)
	# Globalize the path before saving (matches the working checkpoint
	# capture path); save_png on a raw user:// / relative path can silently
	# fail and leave zero frames.
	var path := _globalize_local(frame_dir.path_join("frame_%05d.png" % _frame_index))
	# Only advance the index on a successful write so frame_*.png stays
	# contiguous and ordered for the video assembler.
	if image.save_png(path) == OK:
		_frame_index += 1


func _globalize_local(path: String) -> String:
	if path.begins_with("user://") or path.begins_with("res://"):
		return ProjectSettings.globalize_path(path)
	return path
