extends Node

const MAX_SAMPLES := 300_000
const MONITORS := {
	"fps": Performance.TIME_FPS,
	"draw_calls": Performance.RENDER_TOTAL_DRAW_CALLS_IN_FRAME,
	"primitives": Performance.RENDER_TOTAL_PRIMITIVES_IN_FRAME,
	"objects_in_frame": Performance.RENDER_TOTAL_OBJECTS_IN_FRAME,
	"nodes": Performance.OBJECT_NODE_COUNT,
	"orphans": Performance.OBJECT_ORPHAN_NODE_COUNT,
	"objects_total": Performance.OBJECT_COUNT,
	"static_mem_mb": Performance.MEMORY_STATIC,
	"vram_mb": Performance.RENDER_VIDEO_MEM_USED,
	"physics_2d_active": Performance.PHYSICS_2D_ACTIVE_OBJECTS,
}

var active := false
var truncated := false
var frames := 0
var elapsed_sec := 0.0
var started := 0
var previous := 0
var previous_draws := 0
var valid_draws := 0
var pre_draw := 0
var post_draw := 0
var prefix := ""
var output := ""
var samples := {}
var custom := {}


func _ready() -> void:
	process_mode = Node.PROCESS_MODE_ALWAYS
	set_process(false)


func _exit_tree() -> void:
	_disconnect_draw_signals()


func _disconnect_draw_signals() -> void:
	if RenderingServer.frame_pre_draw.is_connected(_before_draw):
		RenderingServer.frame_pre_draw.disconnect(_before_draw)
	if RenderingServer.frame_post_draw.is_connected(_after_draw):
		RenderingServer.frame_post_draw.disconnect(_after_draw)


func command(args: Dictionary) -> Dictionary:
	match args.get("op"):
		"status":
			return {"result": _status()}
		"start":
			if active:
				return _error("Performance sampling is already active")
			prefix = str(args.get("prefix", ""))
			output = str(args.get("out", ""))
			started = Time.get_ticks_usec()
			previous = 0
			truncated = false
			frames = 0
			valid_draws = 0
			active = true
			samples = {"frame_ms": [], "pre_ms": [], "draw_ms": [], "post_ms": []}
			custom = {}
			RenderingServer.frame_pre_draw.connect(_before_draw)
			RenderingServer.frame_post_draw.connect(_after_draw)
			set_process(true)
			return {"result": _status()}
		"stop":
			if not active:
				return _error("Performance sampling is inactive")
			if not truncated:
				elapsed_sec = (Time.get_ticks_usec() - started) / 1e6
			active = false
			set_process(false)
			_disconnect_draw_signals()
			var report := _summary()
			samples.clear()
			custom.clear()
			var path := str(args.get("out", output))
			if not path.is_empty():
				var err := _write_report(ProjectSettings.globalize_path(path), report)
				if err != OK:
					return _error("Cannot write performance report to %s: %s" % [path, error_string(err)])
			return {"result": report}
	return _error("Unknown performance sampling operation")


func _write_report(path: String, report: Dictionary) -> Error:
	var err := DirAccess.make_dir_recursive_absolute(path.get_base_dir())
	if err != OK:
		return err
	var temp := path + ".tmp"
	var file := FileAccess.open(temp, FileAccess.WRITE)
	if file == null:
		return FileAccess.get_open_error()
	file.store_string(JSON.stringify(report, "\t"))
	file.flush()
	err = file.get_error()
	file.close()
	if err == OK:
		err = DirAccess.rename_absolute(temp, path)
	if err != OK:
		DirAccess.remove_absolute(temp)
	return err


func _error(message: String) -> Dictionary:
	return {"status": "error", "message": message, "result": null}


func _status() -> Dictionary:
	return {"active": active, "frames": frames, "truncated": truncated,
		"elapsed_sec": (Time.get_ticks_usec() - started) / 1e6 if active and not truncated else elapsed_sec}


func _before_draw() -> void:
	pre_draw = Time.get_ticks_usec()


func _after_draw() -> void:
	post_draw = Time.get_ticks_usec()


func _process(_delta: float) -> void:
	var now := Time.get_ticks_usec()
	var drawn := Engine.get_frames_drawn()
	if previous > 0:
		frames += 1
		if drawn - previous_draws == 1:
			valid_draws += 1
		samples.frame_ms.append((now - previous) / 1000.0)
		if pre_draw >= previous and post_draw >= pre_draw:
			samples.pre_ms.append((pre_draw - previous) / 1000.0)
			samples.draw_ms.append((post_draw - pre_draw) / 1000.0)
			samples.post_ms.append((now - post_draw) / 1000.0)
		for key: String in MONITORS:
			var value := Performance.get_monitor(MONITORS[key])
			_record(samples, key, value / 1048576.0 if key.ends_with("_mb") else value)
		_record(samples, "nav_2d_agents", NavigationServer2D.get_process_info(NavigationServer2D.INFO_AGENT_COUNT))
		for key: StringName in Performance.get_custom_monitor_names():
			if str(key).begins_with(prefix):
				var value: Variant = Performance.get_custom_monitor(key)
				if (value is float or value is int or value is bool) and is_finite(float(value)):
					_record(custom, key, float(value))
		if frames == MAX_SAMPLES:
			truncated = true
			elapsed_sec = (now - started) / 1e6
			set_process(false)
			_disconnect_draw_signals()
	previous = now
	previous_draws = drawn
	pre_draw = 0
	post_draw = 0


func _record(series: Dictionary, key: StringName, value: float) -> void:
	if not series.has(key):
		series[key] = []
	series[key].append(value)


func _stats(values: Array) -> Dictionary:
	if values.is_empty():
		return {}
	values.sort()
	var n := values.size()
	return {"mean": values.reduce(func(a, b): return a + b, 0.0) / n,
		"p50": values[ceili(0.50 * n) - 1], "p95": values[ceili(0.95 * n) - 1],
		"p99": values[ceili(0.99 * n) - 1], "max": values.back()}


func _summary() -> Dictionary:
	var report := _status()
	report.valid_draw_pct = 100.0 * valid_draws / frames if frames else 0.0
	report.valid_draw_fps = valid_draws / elapsed_sec if elapsed_sec > 0 else 0.0
	for key in samples:
		report[key] = _stats(samples[key])
	for budget: float in [16.0, 16.7, 33.3]:
		var over := 0
		for ms: float in samples.frame_ms:
			if ms > budget:
				over += 1
		report["over_%sms_pct" % ("%.1f" % budget).replace(".", "_")] = 100.0 * over / frames if frames else 0.0
	report.custom_monitors = {}
	for key in custom:
		report.custom_monitors[key] = _stats(custom[key])
	return report
