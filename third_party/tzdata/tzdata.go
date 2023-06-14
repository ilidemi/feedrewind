// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tzdata

import (
	"fmt"
	"strings"
	"time"
)

var LocationByName map[string]*time.Location

func init() {
	const (
		zecheader = 0x06054b50
		zcheader  = 0x02014b50
		ztailsize = 22

		zheadersize = 30
		zheader     = 0x04034b50
	)

	LocationByName = make(map[string]*time.Location)

	z := zipdata

	idx := len(z) - ztailsize
	n := get2s(z[idx+10:])
	idx = get4s(z[idx+16:])

	for i := 0; i < n; i++ {
		// See time.loadTzinfoFromZip for zip entry layout.
		if get4s(z[idx:]) != zcheader {
			fmt.Println(idx)
			fmt.Println("break")
			break
		}
		meth := get2s(z[idx+10:])
		size := get4s(z[idx+24:])
		namelen := get2s(z[idx+28:])
		xlen := get2s(z[idx+30:])
		fclen := get2s(z[idx+32:])
		off := get4s(z[idx+42:])
		zname := z[idx+46 : idx+46+namelen]
		idx += 46 + namelen + xlen + fclen
		if strings.HasSuffix(zname, "/") {
			continue
		}
		if meth != 0 {
			panic("unsupported compression for " + zname + " in embedded tzdata")
		}

		// See time.loadTzinfoFromZip for zip per-file header layout.
		idx2 := off
		if get4s(z[idx2:]) != zheader ||
			get2s(z[idx2+8:]) != meth ||
			get2s(z[idx2+26:]) != namelen ||
			z[idx2+30:idx2+30+namelen] != zname {
			panic("corrupt embedded tzdata")
		}
		xlen = get2s(z[idx2+28:])
		idx2 += 30 + namelen + xlen
		tzData := z[idx2 : idx2+size]
		location, err := time.LoadLocationFromTZData(zname, []byte(tzData))
		if err != nil {
			panic(err)
		}
		LocationByName[zname] = location
	}
}

// get4s returns the little-endian 32-bit value at the start of s.
func get4s(s string) int {
	if len(s) < 4 {
		return 0
	}
	return int(s[0]) | int(s[1])<<8 | int(s[2])<<16 | int(s[3])<<24
}

// get2s returns the little-endian 16-bit value at the start of s.
func get2s(s string) int {
	if len(s) < 2 {
		return 0
	}
	return int(s[0]) | int(s[1])<<8
}
