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
		}
		return HashValue(entries), nil
	}}
	class.classMethods["interval_ratio"] = &method{owner: class, name: "interval_ratio", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(0), nil
	}}
	class.classMethods["step_ratio"] = &method{owner: class, name: "step_ratio", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(0), nil
	}}
	class.classMethods["step_limit"] = &method{owner: class, name: "step_limit", native: func(ctx context.Context, self Value, args []Value, kw Value, block Value) (Value, error) {
		return IntValue(0), nil
	}}
}
