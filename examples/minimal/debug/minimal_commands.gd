extends ConsoleCommandModule


func build() -> Array:
	return [
		Spec.CommandSpec.flat("ping", "Check that the game console is responsive", [], Callable(self, "_ping")),
		Spec.CommandSpec.flat(
			"echo",
			"Return text through the runtime bridge",
			[Spec.ParamSpec.required("text", Types.EXPRESSION)],
			Callable(self, "_echo")
		),
	]


func _ping(_context: Object, _args: Dictionary) -> String:
	return "pong"


func _echo(_context: Object, args: Dictionary) -> String:
	return String(args.text)
