// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3 with static-linking exception.
// See LICENCE file for details.

package tcglog

import (
	"bytes"
	"testing"

	"github.com/canonical/go-efilib"
)

func TestEFIVariableDataEncode(t *testing.T) {
	for _, data := range []struct {
		desc string
		in   EFIVariableData
		out  []byte
	}{
		{
			desc: "db",
			in: EFIVariableData{
				VariableName: efi.MakeGUID(0xd719b2cb, 0x3d3a, 0x4596, 0xa3bc, [...]uint8{0xda, 0xd0, 0x0e, 0x67, 0x65, 0x6f}),
				UnicodeName:  "db",
				VariableData: []byte("foo")},
			out: []byte{0xcb, 0xb2, 0x19, 0xd7, 0x3a, 0x3d, 0x96, 0x45, 0xa3, 0xbc, 0xda, 0xd0, 0x0e,
				0x67, 0x65, 0x6f, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x64, 0x00, 0x62, 0x00, 0x66, 0x6f, 0x6f},
		},
		{
			desc: "dbx",
			in: EFIVariableData{
				VariableName: efi.MakeGUID(0xd719b2cb, 0x3d3a, 0x4596, 0xa3bc, [...]uint8{0xda, 0xd0, 0x0e, 0x67, 0x65, 0x6f}),
				UnicodeName:  "dbx",
				VariableData: []byte("bar")},
			out: []byte{0xcb, 0xb2, 0x19, 0xd7, 0x3a, 0x3d, 0x96, 0x45, 0xa3, 0xbc, 0xda, 0xd0, 0x0e,
				0x67, 0x65, 0x6f, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x64, 0x00, 0x62, 0x00, 0x78, 0x00, 0x62, 0x61,
				0x72},
		},
	} {
		t.Run(data.desc, func(t *testing.T) {
			var buf bytes.Buffer
			if err := data.in.EncodeMeasuredBytes(&buf); err != nil {
				t.Fatalf("Encode failed: %v", err)
			}
			if !bytes.Equal(buf.Bytes(), data.out) {
				t.Errorf("Unexpected encoding")
			}
		})
	}
}
