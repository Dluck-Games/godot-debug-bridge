# ParamType — the exhaustive set of argument categories understood by the
# console dispatcher. Each value must have a matching branch in
# console_completer.gd and in the validation logic in console_registry.gd.
class_name ConsoleParamType
extends RefCounted


enum {
	STRING,       # raw string, no validation or completion
	INT,          # parsed with is_valid_int / to_int
	FLOAT,        # parsed with is_valid_float / to_float
	BOOL_TOGGLE,  # on/off/true/false/1/0; empty permitted if handler treats as toggle
	ENUM,         # value must be in spec.enum_values
	EXPRESSION,   # trailing-slurp string (consumes all remaining tokens)
	CUSTOM_BASE = 100,  # projects may define stable types from this value upward
}
