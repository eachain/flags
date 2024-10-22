package flags

import (
	"context"
	"testing"
	"time"
)

func TestHandle(t *testing.T) {
	fs := New("handle", "")
	var run bool
	fs.Handle(func(context.Context) { run = true })
	_, err := fs.Run(context.Background())
	if err != nil {
		t.Fatalf("handle run: %v", err)
	}
	if !run {
		t.Fatal("handle: not run")
	}
}

func TestUse(t *testing.T) {
	fs := New("use", "")
	var run [6]bool
	fs.Use(func(ctx context.Context, next Handler) {
		run[0] = true
		next(ctx)
	})

	fs.Use(func(ctx context.Context, next Handler) {
		run[1] = true
		next(ctx)
	}, func(ctx context.Context, next Handler) {
		run[2] = true
		next(ctx)
	})

	fs.Cmd("sub", "", func(ctx context.Context, next Handler) {
		run[3] = true
		next(ctx)
	}).Use(func(ctx context.Context, next Handler) {
		run[4] = true
		next(ctx)
	}).Handle(func(context.Context) { run[5] = true })

	_, err := fs.Run(context.Background(), "sub")
	if err != nil {
		t.Fatalf("use run: %v", err)
	}
	for i, r := range run {
		if !r {
			t.Fatalf("use: %vth not run: %v", i+1, run)
		}
	}
}

func TestNumber(t *testing.T) {
	var i int
	fs := New("number", "")
	fs.IntVar(&i, 'i', "int", 789, "a number value")

	// default value
	fs.Handle(func(context.Context) {
		if i != 789 {
			t.Fatalf("number run result: %v", i)
		}
	})
	_, err := fs.Run(context.Background())
	if err != nil {
		t.Fatalf("number run: %v", err)
	}

	// short
	fs.Handle(func(context.Context) {
		if i != 123 {
			t.Fatalf("number run result: %v", i)
		}
	})
	_, err = fs.Run(context.Background(), "-i", "123")
	if err != nil {
		t.Fatalf("number run: %v", err)
	}

	// long
	fs.Handle(func(context.Context) {
		if i != 456 {
			t.Fatalf("number run result: %v", i)
		}
	})
	_, err = fs.Run(context.Background(), "--int", "456")
	if err != nil {
		t.Fatalf("number run: %v", err)
	}

	// long align
	fs.Handle(func(context.Context) {
		if i != 789 {
			t.Fatalf("number run result: %v", i)
		}
	})
	_, err = fs.Run(context.Background(), "--int=789")
	if err != nil {
		t.Fatalf("number run: %v", err)
	}
}

func TestBool(t *testing.T) {
	var b bool
	fs := New("bool", "")
	fs.BoolVar(&b, 'b', "bool", false, "a bool value")

	// default
	fs.Handle(func(context.Context) {
		if b {
			t.Fatalf("bool run result: %v", b)
		}
	})
	_, err := fs.Run(context.Background())
	if err != nil {
		t.Fatalf("bool run: %v", err)
	}

	// short
	fs.Handle(func(context.Context) {
		if !b {
			t.Fatalf("bool run result: %v", b)
		}
	})
	_, err = fs.Run(context.Background(), "-b")
	if err != nil {
		t.Fatalf("bool run: %v", err)
	}

	// long
	b = false
	fs.Handle(func(context.Context) {
		if !b {
			t.Fatalf("bool run result: %v", b)
		}
	})
	_, err = fs.Run(context.Background(), "--bool")
	if err != nil {
		t.Fatalf("bool run: %v", err)
	}

	// long align
	b = false
	fs.Handle(func(context.Context) {
		if !b {
			t.Fatalf("bool run result: %v", b)
		}
	})
	_, err = fs.Run(context.Background(), "--bool=true")
	if err != nil {
		t.Fatalf("bool run: %v", err)
	}

	// invalid format:
	_, err = fs.Run(context.Background(), "-b false")
	if err == nil {
		t.Fatalf("bool run: no err")
	}

	// when default is true, the only way set it false:
	fs = New("bool", "")
	fs.BoolVar(&b, 'b', "bool", true, "a bool value")
	b = true
	fs.Handle(func(context.Context) {
		if b {
			t.Fatalf("bool run result: %v", b)
		}
	})
	_, err = fs.Run(context.Background(), "--bool=false")
	if err != nil {
		t.Fatalf("bool run: %v", err)
	}

	// default is true:
	fs = New("bool", "")
	fs.BoolVar(&b, 'b', "bool", true, "a bool value")
	b = false
	fs.Handle(func(context.Context) {
		if !b {
			t.Fatalf("bool run result: %v", b)
		}
	})
	_, err = fs.Run(context.Background())
	if err != nil {
		t.Fatalf("bool run: %v", err)
	}
}

func TestDuration(t *testing.T) {
	var d time.Duration
	fs := New("duration", "")
	fs.DurationVar(&d, 'd', "dur", time.Second, "a duration value")

	// default
	fs.Handle(func(context.Context) {
		if d != time.Second {
			t.Fatalf("duration run result: %v", d)
		}
	})
	_, err := fs.Run(context.Background())
	if err != nil {
		t.Fatalf("duration run: %v", err)
	}

	// short
	fs.Handle(func(context.Context) {
		if d != 2*time.Second {
			t.Fatalf("duration run result: %v", d)
		}
	})
	_, err = fs.Run(context.Background(), "-d", "2s")
	if err != nil {
		t.Fatalf("duration run: %v", err)
	}

	// long
	fs.Handle(func(context.Context) {
		if d != 3*time.Second {
			t.Fatalf("duration run result: %v", d)
		}
	})
	_, err = fs.Run(context.Background(), "--dur", "3s")
	if err != nil {
		t.Fatalf("duration run: %v", err)
	}

	// long align
	fs.Handle(func(context.Context) {
		if d != 9*time.Second {
			t.Fatalf("duration run result: %v", d)
		}
	})
	_, err = fs.Run(context.Background(), "--dur=9s")
	if err != nil {
		t.Fatalf("duration run: %v", err)
	}
}

func TestDateTime(t *testing.T) {
	var d time.Time
	fs := New("datetime", "")

	dft, _ := time.ParseInLocation(DateTime, "2024-01-02T15:04:05", time.Local)
	fs.DateTimeVar(&d, 't', "time", dft, "a datetime value")

	// default
	fs.Handle(func(context.Context) {
		if !d.Equal(dft) {
			t.Fatalf("datetime run result: %v", d)
		}
	})
	_, err := fs.Run(context.Background())
	if err != nil {
		t.Fatalf("datetime run: %v", err)
	}

	// short
	fs = New("datetime", "")
	fs.DateTimeVar(&d, 't', "time", time.Time{}, "a datetime value")
	now := time.Now().Truncate(time.Second)
	fs.Handle(func(context.Context) {
		if !d.Equal(now) {
			t.Fatalf("datetime run result: %v", d)
		}
	})
	_, err = fs.Run(context.Background(), "-t", now.Format(DateTime))
	if err != nil {
		t.Fatalf("datetime run: %v", err)
	}

	// long
	minute := time.Now().Truncate(time.Minute)
	fs.Handle(func(context.Context) {
		if !d.Equal(minute) {
			t.Fatalf("datetime run result: %v", d)
		}
	})
	_, err = fs.Run(context.Background(), "--time", minute.Format(DateTime))
	if err != nil {
		t.Fatalf("datetime run: %v", err)
	}

	// long align
	hour := time.Now().Truncate(time.Hour)
	fs.Handle(func(context.Context) {
		if !d.Equal(hour) {
			t.Fatalf("datetime run result: %v", d)
		}
	})
	_, err = fs.Run(context.Background(), "--time="+hour.Format(DateTime))
	if err != nil {
		t.Fatalf("datetime run: %v", err)
	}
}

func sliceEqual[T comparable](s []T, t ...T) bool {
	if len(s) != len(t) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != t[i] {
			return false
		}
	}
	return true
}

func TestSliceVar(t *testing.T) {
	var s []int64
	fs := New("slice", "")
	SliceVar(fs, &s, 's', "slice", []int64{-1, -2, -3}, "a slice of number")

	// default
	fs.Handle(func(context.Context) {
		if !sliceEqual(s, -1, -2, -3) {
			t.Fatalf("slice run result: %v", s)
		}
	})
	_, err := fs.Run(context.Background())
	if err != nil {
		t.Fatalf("slice run: %v", err)
	}

	// short & long
	s = s[:0]
	fs.Handle(func(context.Context) {
		if !sliceEqual(s, 4, -5, 6) {
			t.Fatalf("slice run result: %v", s)
		}
	})
	_, err = fs.Run(context.Background(), "-s", "4", "--slice", "-5", "-s", "6")
	if err != nil {
		t.Fatalf("slice run: %v", err)
	}

	// long align
	s = s[:0]
	fs.Handle(func(context.Context) {
		if !sliceEqual(s, -7, 8, -9) {
			t.Fatalf("slice run result: %v", s)
		}
	})
	_, err = fs.Run(context.Background(), "--slice=-7,8,-9")
	if err != nil {
		t.Fatalf("slice run: %v", err)
	}
}

func mapEqual[K, V comparable](m1, m2 map[K]V) bool {
	for k, v := range m1 {
		if m2[k] != v {
			return false
		}
	}
	for k, v := range m2 {
		if m1[k] != v {
			return false
		}
	}
	return true
}

func TestMapVar(t *testing.T) {
	var m map[string]uint64
	fs := New("map", "")
	MapVar(fs, &m, 'm', "map", map[string]uint64{"a": 1, "b": 2}, "a map of string number")

	// default
	fs.Handle(func(context.Context) {
		if !mapEqual(m, map[string]uint64{"a": 1, "b": 2}) {
			t.Fatalf("map run result: %v", m)
		}
	})
	_, err := fs.Run(context.Background())
	if err != nil {
		t.Fatalf("map run: %v", err)
	}

	// short & long
	m = nil
	fs.Handle(func(context.Context) {
		if !mapEqual(m, map[string]uint64{"a": 7, "b": 8, "c": 9, "x": 11, "y": 22, "z": 33}) {
			t.Fatalf("map run result: %v", m)
		}
	})
	_, err = fs.Run(context.Background(), "--map", "a:7,b:8,c:9", "-m", "x:11,y:22,z:33")
	if err != nil {
		t.Fatalf("map run: %v", err)
	}

	// long align
	m = nil
	fs.Handle(func(context.Context) {
		if !mapEqual(m, map[string]uint64{"x": 123, "y": 456, "z": 789}) {
			t.Fatalf("map run result: %v", m)
		}
	})
	_, err = fs.Run(context.Background(), "--map=x:123,y:456", "--map=z:789")
	if err != nil {
		t.Fatalf("map run: %v", err)
	}
}

func sliceMapEqual[K, V comparable](s1, s2 []map[K]V) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := range s1 {
		if !mapEqual(s1[i], s2[i]) {
			return false
		}
	}
	return true
}

func TestSliceMap(t *testing.T) {
	var sm []map[string]time.Duration
	fs := New("slice_map", "")
	fs.AnyVar(&sm, 's', "sm", []map[string]time.Duration{{"a": 1, "b": 2}}, "a slice map of string duration")

	// default
	fs.Handle(func(context.Context) {
		if !sliceMapEqual(sm, []map[string]time.Duration{{"a": 1, "b": 2}}) {
			t.Fatalf("slice_map run result: %v", sm)
		}
	})
	_, err := fs.Run(context.Background())
	if err != nil {
		t.Fatalf("slice_map run: %v", err)
	}

	// short
	sm = nil
	fs.Handle(func(context.Context) {
		if !sliceMapEqual(sm, []map[string]time.Duration{{"x": time.Second, "y": 2 * time.Minute}, {"z": 3 * time.Hour}}) {
			t.Fatalf("slice_map run result: %v", sm)
		}
	})
	_, err = fs.Run(context.Background(), "-s", "x:1s,y:2m", "-s", "z:3h")
	if err != nil {
		t.Fatalf("slice_map run: %v", err)
	}

	// long
	sm = nil
	fs.Handle(func(context.Context) {
		if !sliceMapEqual(sm, []map[string]time.Duration{{"x": time.Hour}, {"y": 2 * time.Minute, "z": 3*time.Hour + 4*time.Minute + 5*time.Second}}) {
			t.Fatalf("slice_map run result: %v", sm)
		}
	})
	_, err = fs.Run(context.Background(), "--sm", "x:1h", "--sm", "y:2m,z:3h4m5s")
	if err != nil {
		t.Fatalf("slice_map run: %v", err)
	}

	// long align
	sm = nil
	fs.Handle(func(context.Context) {
		if !sliceMapEqual(sm, []map[string]time.Duration{{"x": 9 * time.Minute}, {"y": 8 * time.Second, "z": 7*time.Hour + 6*time.Minute + 5*time.Second}}) {
			t.Fatalf("slice_map run result: %v", sm)
		}
	})
	_, err = fs.Run(context.Background(), "--sm=x:9m", "--sm=y:8s,z:7h6m5s")
	if err != nil {
		t.Fatalf("slice_map run: %v", err)
	}
}

func mapSliceEqual[K, V comparable](m1, m2 map[K][]V) bool {
	for k := range m1 {
		if !sliceEqual(m1[k], m2[k]...) {
			return false
		}
	}
	for k := range m2 {
		if !sliceEqual(m1[k], m2[k]...) {
			return false
		}
	}
	return true
}

func TestMapSlice(t *testing.T) {
	var ms map[uint8][]string
	fs := New("map_slice", "")
	fs.AnyVar(&ms, 'm', "ms", map[uint8][]string{1: {"a"}, 2: {"b"}}, "a map of uint8 []string")

	// default
	fs.Handle(func(context.Context) {
		if !mapSliceEqual(ms, map[uint8][]string{1: {"a"}, 2: {"b"}}) {
			t.Fatalf("map_slice run result: %v", ms)
		}
	})
	_, err := fs.Run(context.Background())
	if err != nil {
		t.Fatalf("map_slice run: %v", err)
	}

	// short
	ms = nil
	fs.Handle(func(context.Context) {
		if !mapSliceEqual(ms, map[uint8][]string{11: {"x", "y"}, 22: {"z"}}) {
			t.Fatalf("map_slice run result: %v", ms)
		}
	})
	_, err = fs.Run(context.Background(), "-m", "11:x,11:y", "-m", "22:z")
	if err != nil {
		t.Fatalf("map_slice run: %v", err)
	}

	// long
	ms = nil
	fs.Handle(func(context.Context) {
		if !mapSliceEqual(ms, map[uint8][]string{99: {"x"}, 88: {"y", "z"}}) {
			t.Fatalf("map_slice run result: %v", ms)
		}
	})
	_, err = fs.Run(context.Background(), "--ms", "99:x,88:y", "--ms", "88:z")
	if err != nil {
		t.Fatalf("map_slice run: %v", err)
	}

	// long align
	ms = nil
	fs.Handle(func(context.Context) {
		if !mapSliceEqual(ms, map[uint8][]string{7: {"a", "b"}, 6: {"x", "y", "z"}}) {
			t.Fatalf("map_slice run result: %v", ms)
		}
	})
	_, err = fs.Run(context.Background(), "--ms=7:a,7:b,6:x", "--ms=6:y,6:z")
	if err != nil {
		t.Fatalf("map_slice run: %v", err)
	}
}
