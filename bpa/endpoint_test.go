package bpa

import (
	"reflect"
	"testing"
)

func TestEndpointDtnNone(t *testing.T) {
	dtnNone, err := NewEndpointID("dtn", "none")

	if err != nil {
		t.Errorf("dtn:none resulted in an error: %v", err)
	}

	if dtnNone.SchemeName != URISchemeDTN {
		t.Errorf("dtn:none has wrong scheme name: %d", dtnNone.SchemeName)
	}
	if ty := reflect.TypeOf(dtnNone.SchemeSpecificPort); ty.Kind() != reflect.Uint {
		t.Errorf("dtn:none's SSP has wrong type: %T instead of uint", ty)
	}
	if v := dtnNone.SchemeSpecificPort.(uint); v != 0 {
		t.Errorf("dtn:none's SSP is not 0, %d", v)
	}

	if str := dtnNone.String(); str != "dtn:none" {
		t.Errorf("dtn:none's string representation is %v", str)
	}
}

func TestEndpointDtn(t *testing.T) {
	dtnEP, err := NewEndpointID("dtn", "foobar")

	if err != nil {
		t.Errorf("dtn:foobar resulted in an error: %v", err)
	}

	if dtnEP.SchemeName != URISchemeDTN {
		t.Errorf("dtn:foobar has wrong scheme name: %d", dtnEP.SchemeName)
	}
	if ty := reflect.TypeOf(dtnEP.SchemeSpecificPort); ty.Kind() != reflect.String {
		t.Errorf("dtn:foobar's SSP has wrong type: %T instead of string", ty)
	}
	if v := dtnEP.SchemeSpecificPort.(string); v != "foobar" {
		t.Errorf("dtn:foobar's SSP is not 'foobar', %v", v)
	}

	if str := dtnEP.String(); str != "dtn:foobar" {
		t.Errorf("dtn:foobar's string representation is %v", str)
	}
}

func TestEndpointIpn(t *testing.T) {
	ipnEP, err := NewEndpointID("ipn", "23.42")

	if err != nil {
		t.Errorf("ipn:23.42 resulted in an error: %v", err)
	}

	if ipnEP.SchemeName != URISchemeIPN {
		t.Errorf("ipn:23.42 has wrong scheme name: %d", ipnEP.SchemeName)
	}
	if ty := reflect.TypeOf(ipnEP.SchemeSpecificPort); ty.Kind() == reflect.Array {
		if te := ty.Elem(); te.Kind() != reflect.Uint64 {
			t.Errorf("ipn:23.42's SSP array has wrong elem-type: %T instead of uint64", te)
		}
	} else {
		t.Errorf("ipn:23.42's SSP has wrong type: %T instead of array", ty)
	}
	if v := ipnEP.SchemeSpecificPort.([2]uint64); len(v) == 2 {
		if v[0] != 23 && v[1] != 42 {
			t.Errorf("ipn:23.42's SSP != (23, 42); (%d, %d)", v[0], v[1])
		}
	} else {
		t.Errorf("ipn:23.42's SSP length is not two, %d", len(v))
	}

	if str := ipnEP.String(); str != "ipn:23.42" {
		t.Errorf("ipn:23.42's string representation is %v", str)
	}
}

func TestEndpointIpnInvalid(t *testing.T) {
	testCases := []string{
		// Wrong regular expression:
		"23.", "23", ".23", "-10.5", "10.-3", "", "foo.bar", "0x23.0x42",
		// Too small numbers
		"0.23", "23.0",
		// Too big numbers
		"23.18446744073709551616", "18446744073709551616.23",
		"23.99999999999999999999", "99999999999999999999.23",
	}

	for _, testCase := range testCases {
		_, err := NewEndpointID("ipn", testCase)
		if err == nil {
			t.Errorf("ipn:%v does not resulted in an error", testCase)
		}
	}
}

func TestEndpointInvalid(t *testing.T) {
	testCases := []struct {
		name string
		ssp  string
	}{
		{"foo", "bar"},
	}

	for _, testCase := range testCases {
		_, err := NewEndpointID(testCase.name, testCase.ssp)
		if err == nil {
			t.Errorf("%v:%v does not resulted in an error", testCase.name, testCase.ssp)
		}
	}
}