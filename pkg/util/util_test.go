/*
Copyright 2021 Juicedata Inc

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"errors"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	. "github.com/agiledragon/gomonkey/v2"
	. "github.com/smartystreets/goconvey/convey"
)

func TestContainsString(t *testing.T) {
	type args struct {
		slice []string
		s     string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "test-true",
			args: args{
				slice: []string{"a", "b"},
				s:     "a",
			},
			want: true,
		},
		{
			name: "test-false",
			args: args{
				slice: []string{"a", "b"},
				s:     "c",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsString(tt.args.slice, tt.args.s); got != tt.want {
				t.Errorf("ContainsString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseEndpoint(t *testing.T) {
	type args struct {
		endpoint string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		want1   string
		wantErr bool
	}{
		{
			name: "test",
			args: args{
				endpoint: "unix://tmp/csi.sock",
			},
			want:    "unix",
			want1:   "/tmp/csi.sock",
			wantErr: false,
		},
		{
			name: "test-error",
			args: args{
				endpoint: "http://test",
			},
			want:    "",
			want1:   "",
			wantErr: true,
		},
		{
			name: "test-nil",
			args: args{
				endpoint: "",
			},
			want:    "",
			want1:   "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1, err := ParseEndpoint(tt.args.endpoint)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEndpoint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseEndpoint() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("ParseEndpoint() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func TestParseEndpointError(t *testing.T) {
	Convey("Test ParseEndpoint", t, func() {
		Convey("parse error", func() {
			patch1 := ApplyFunc(url.Parse, func(rawURL string) (*url.URL, error) {
				return nil, errors.New("test")
			})
			defer patch1.Reset()
			_, _, err := ParseEndpoint("unix://tmp/csi.sock")
			So(err, ShouldNotBeNil)
		})
		Convey("not exist", func() {
			patch1 := ApplyFunc(os.IsNotExist, func(err error) bool {
				return false
			})
			defer patch1.Reset()
			patch2 := ApplyFunc(os.Remove, func(addr string) error {
				return errors.New("test")
			})
			defer patch2.Reset()
			_, _, err := ParseEndpoint("unix://tmp/csi.sock")
			So(err, ShouldNotBeNil)
		})
	})
}

func TestGetReferenceKey(t *testing.T) {
	type args struct {
		target string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test",
			args: args{
				target: "test",
			},
			want: "juicefs-9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c1",
		},
		{
			name: "test-nil",
			args: args{
				target: "",
			},
			want: "juicefs-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetReferenceKey(tt.args.target); got != tt.want {
				t.Errorf("GetReferenceKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTimeAfterDelay(t *testing.T) {
	now := time.Now()
	type args struct {
		delayStr string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "test",
			args: args{
				delayStr: "1h",
			},
			want:    now.UTC().Add(1 * time.Hour).Format(time.RFC3339),
			wantErr: false,
		},
		{
			name: "test-err",
			args: args{
				delayStr: "1hour",
			},
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetTimeAfterDelay(tt.args.delayStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTimeAfterDelay() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetTimeAfterDelay() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTime(t *testing.T) {
	tests := []struct {
		name    string
		str     string
		want    time.Time
		wantErr bool
	}{
		{
			name: "rfc3339-utc",
			str:  "2026-05-29T12:20:38Z",
			want: time.Date(2026, 5, 29, 12, 20, 38, 0, time.UTC),
		},
		{
			name: "rfc3339-offset",
			str:  "2026-05-29T20:20:38+08:00",
			want: time.Date(2026, 5, 29, 12, 20, 38, 0, time.UTC),
		},
		{
			name:    "invalid",
			str:     "not-a-time",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetTime(tt.str)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTime() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("GetTime() got = %v, want (same instant) %v", got, tt.want)
			}
		})
	}
}

// TestGetTimeLegacyFormat verifies backward compatibility: the legacy zone-less
// "2006-01-02 15:04:05" value was written using local time, so GetTime must
// parse it in the local location (not UTC), otherwise existing delete-at
// annotations would be shifted by the local UTC offset.
func TestGetTimeLegacyFormat(t *testing.T) {
	orig := time.Local
	defer func() { time.Local = orig }()
	time.Local = time.FixedZone("CST", 8*3600) // UTC+8

	got, err := GetTime("2026-05-29 12:20:38")
	if err != nil {
		t.Fatalf("GetTime legacy error: %v", err)
	}
	wantLocal := time.Date(2026, 5, 29, 12, 20, 38, 0, time.Local) // 12:20:38 CST
	if !got.Equal(wantLocal) {
		t.Errorf("legacy value parsed as %v, want %v (parsed in local zone, not UTC)", got, wantLocal)
	}
}

// TestGetTimeAfterDelayRoundTrip guards against a writer/reader timezone
// mismatch: GetTimeAfterDelay writes the timestamp and GetTime reads it back,
// so GetTime(GetTimeAfterDelay(d)) must be ~d in the future regardless of the
// process's local timezone. Before the fix, GetTimeAfterDelay formatted local
// time while GetTime parsed it as UTC, shifting the result by the local UTC
// offset (e.g. +8h in CST).
func TestGetTimeAfterDelayRoundTrip(t *testing.T) {
	orig := time.Local
	defer func() { time.Local = orig }()

	for _, tz := range []*time.Location{
		time.UTC,
		time.FixedZone("CST", 8*3600),  // UTC+8
		time.FixedZone("EST", -5*3600), // UTC-5
	} {
		time.Local = tz
		const delay = 3 * time.Hour
		s, err := GetTimeAfterDelay(delay.String())
		if err != nil {
			t.Fatalf("tz=%s GetTimeAfterDelay error: %v", tz, err)
		}
		at, err := GetTime(s)
		if err != nil {
			t.Fatalf("tz=%s GetTime error: %v", tz, err)
		}
		got := time.Until(at)
		if diff := got - delay; diff < -2*time.Second || diff > 2*time.Second {
			t.Errorf("tz=%s round-trip delay = %v, want ~%v (off by %v); writer/reader timezone mismatch", tz, got, delay, diff)
		}
	}
}

func Test_quoteForShell(t *testing.T) {
	type args struct {
		cmd string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test-(",
			args: args{
				cmd: "mysql://user@(127.0.0.1:3306)/juicefs",
			},
			want: "mysql://user@\\(127.0.0.1:3306\\)/juicefs",
		},
		{
			name: "test-none",
			args: args{
				cmd: "redis://127.0.0.1:6379/0",
			},
			want: "redis://127.0.0.1:6379/0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := QuoteForShell(tt.args.cmd); got != tt.want {
				t.Errorf("transformCmd() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStripPasswd(t *testing.T) {
	type args struct {
		uri string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "test1",
			args: args{
				uri: "redis://:abc@127.0.0.1:6379/0",
			},
			want: "redis://:****@127.0.0.1:6379/0",
		},
		{
			name: "test2",
			args: args{
				uri: "redis://127.0.0.1:6379/0",
			},
			want: "redis://127.0.0.1:6379/0",
		},
		{
			name: "test3",
			args: args{
				uri: "redis://abc:abc@127.0.0.1:6379/0",
			},
			want: "redis://abc:****@127.0.0.1:6379/0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripPasswd(tt.args.uri); got != tt.want {
				t.Errorf("StripPasswd() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckDynamicPV(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "test-true",
			args: args{
				name: "pvc-95aba554-3fe4-4433-9d25-d2a63a114367",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "test-false",
			args: args{
				name: "test",
			},
			want:    false,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CheckDynamicPV(tt.args.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckDynamicPV() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("CheckDynamicPV() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainsPrefix(t *testing.T) {
	type args struct {
		slice []string
		s     string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "test-true",
			args: args{
				slice: []string{"metrics=0.0.0.0:9567", "subdir=/metrics"},
				s:     "metrics=",
			},
			want: true,
		},
		{
			name: "test-false",
			args: args{
				slice: []string{"subdir=/metrics"},
				s:     "metrics=",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsPrefix(tt.args.slice, tt.args.s); got != tt.want {
				t.Errorf("ContainsPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStripReadonlyOption(t *testing.T) {
	type args struct {
		options []string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "test-none",
			args: args{
				options: []string{},
			},
			want: []string{},
		},
		{
			name: "test-ro",
			args: args{
				options: []string{"ro"},
			},
			want: []string{},
		},
		{
			name: "test-read-only",
			args: args{
				options: []string{"read-only"},
			},
			want: []string{},
		},
		{
			name: "test-no-ro",
			args: args{
				options: []string{"subdir=/metrics"},
			},
			want: []string{"subdir=/metrics"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripReadonlyOption(tt.args.options); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("StripReadonlyOption() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckExpectValue(t *testing.T) {
	type args struct {
		m           map[string]string
		key         string
		targetValue string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "test-true",
			args: args{
				m: map[string]string{
					"a": "b",
					"c": "d",
				},
				key:         "a",
				targetValue: "b",
			},
			want: true,
		},
		{
			name: "test-false",
			args: args{
				m: map[string]string{
					"a": "b",
					"c": "d",
				},
				key:         "a",
				targetValue: "c",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckExpectValue(tt.args.m, tt.args.key, tt.args.targetValue); got != tt.want {
				t.Errorf("CheckExpectValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestImageResol(t *testing.T) {
	type args struct {
		image string
	}
	tests := []struct {
		name      string
		args      args
		wantHasCE bool
		wantHasEE bool
	}{
		{
			name: "test-latest",
			args: args{
				image: "juicedata/mount:latest",
			},
			wantHasCE: true,
			wantHasEE: false,
		},
		{
			name: "test-ce",
			args: args{
				image: "juicedata/mount:ce-1.0.0",
			},
			wantHasCE: true,
			wantHasEE: false,
		},
		{
			name: "test-ee",
			args: args{
				image: "juicedata/mount:ee-4.9.0",
			},
			wantHasCE: false,
			wantHasEE: true,
		},
		{
			name: "test-both",
			args: args{
				image: "juicedata/mount:v1.0.0-4.9.0",
			},
			wantHasCE: true,
			wantHasEE: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHasCE, gotHasEE := ImageResol(tt.args.image)
			if gotHasCE != tt.wantHasCE {
				t.Errorf("ImageResol() gotHasCE = %v, want %v", gotHasCE, tt.wantHasCE)
			}
			if gotHasEE != tt.wantHasEE {
				t.Errorf("ImageResol() gotHasEE = %v, want %v", gotHasEE, tt.wantHasEE)
			}
		})
	}
}

func TestParseToBytes(t *testing.T) {
	tests := []struct {
		name    string
		args    string
		want    uint64
		wantErr bool
	}{
		{
			name: "test-normal",
			args: "1",
			want: 1 << 20,
		},
		{
			name: "test-has-uint",
			args: "1M",
			want: 1 << 20,
		},
		{
			name: "test-has-uint-2",
			args: "1G",
			want: 1 << 30,
		},
		{
			name:    "test-invalid",
			args:    "1d",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseToBytes(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBytes() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseBytes() got = %v, want %v", got, tt.want)
			}
		})
	}
}
