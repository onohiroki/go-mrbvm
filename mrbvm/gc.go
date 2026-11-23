package mrbvm

import (
	"context"
	"runtime"
)

func (s *State) installGCLibrary() {
	class := s.GCClass
	class.classMethods["start"] = &method{owner: class, name: "start", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		runtime.GC()
		return Nil, nil
	}}
	class.classMethods["enable"] = &method{owner: class, name: "enable", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		s.gcEnabled = true
		return Nil, nil
	}}
	class.classMethods["disable"] = &method{owner: class, name: "disable", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		s.gcEnabled = false
		return Nil, nil
	}}
	class.classMethods["enable?"] = &method{owner: class, name: "enable?", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(s.gcEnabled), nil
	}}
	class.classMethods["count"] = &method{owner: class, name: "count", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		return IntValue(int64(stats.NumGC)), nil
	}}
	class.classMethods["stat"] = &method{owner: class, name: "stat", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		var stats runtime.MemStats
		runtime.ReadMemStats(&stats)
		entries := []hashEntry{
			{key: SymbolValue("count"), value: IntValue(int64(stats.NumGC))},
			{key: SymbolValue("heap_alloc"), value: IntValue(int64(stats.HeapAlloc))},
			{key: SymbolValue("heap_sys"), value: IntValue(int64(stats.HeapSys))},
			{key: SymbolValue("total_alloc"), value: IntValue(int64(stats.TotalAlloc))},
			{key: SymbolValue("mallocs"), value: IntValue(int64(stats.Mallocs))},
			{key: SymbolValue("frees"), value: IntValue(int64(stats.Frees))},
			{key: SymbolValue("debt"), value: IntValue(0)},
			{key: SymbolValue("live"), value: IntValue(0)},
		}
		return HashValue(entries), nil
	}}
	class.classMethods["interval_ratio"] = &method{owner: class, name: "interval_ratio", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) > 0 {
			return args[0], nil
		}
		return IntValue(0), nil
	}}
	class.classMethods["generational_mode"] = &method{owner: class, name: "generational_mode", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) > 0 {
			return args[0], nil
		}
		return False, nil
	}}
	class.classMethods["generational_mode="] = class.classMethods["generational_mode"]
	class.classMethods["interval_ratio="] = class.classMethods["interval_ratio"]
	class.classMethods["step_ratio"] = &method{owner: class, name: "step_ratio", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(0), nil
	}}
	class.classMethods["step_limit"] = &method{owner: class, name: "step_limit", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(0), nil
	}}
	class.classMethods["generational_mode"] = &method{owner: class, name: "generational_mode", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return BoolValue(s.gcGenerational), nil
	}}
	class.classMethods["generational_mode="] = &method{owner: class, name: "generational_mode=", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		if len(args) != 1 {
			return Nil, argumentCountError(s, len(args), "1")
		}
		s.gcGenerational = args[0].IsTruthy()
		return BoolValue(s.gcGenerational), nil
	}}
}
