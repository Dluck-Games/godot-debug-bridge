# ConsoleCommandModule — base class for each command module under commands/.
# A module is a stateless factory of CommandSpec objects. The registry
# instantiates one instance per module and calls build() once at setup.
# Handlers receive ConsoleContext + Dictionary args.
#
# Lifetime contract: CommandSpec.handler stores Callable(self, "_handler"),
# which holds a weak reference to this module instance. The registry MUST
# retain module instances (see ConsoleRegistry._modules) for as long as the
# specs they produced are reachable — otherwise the Callables silently
# become no-ops when the modules are freed. Do not cache a freshly
# instantiated module locally and discard it; always register via the
# registry's setup() path.
class_name ConsoleCommandModule
extends RefCounted

const Spec := preload("res://addons/gdbg/console/console_command.gd")
const Types := preload("res://addons/gdbg/console/console_types.gd")


func build() -> Array:  # Array[Spec.CommandSpec]
	return []
