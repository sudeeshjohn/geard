package dbus

import (
	"bytes"
	"encoding/binary"
	"io/ioutil"
	"reflect"
	"testing"
)

var protoTests = []struct {
	vs         []interface{}
	marshalled []byte
}{
	{
		[]interface{}{int32(0)},
		[]byte{0, 0, 0, 0},
	},
	{
		[]interface{}{int16(32)},
		[]byte{0, 32},
	},
	{
		[]interface{}{"foo"},
		[]byte{0, 0, 0, 3, 'f', 'o', 'o', 0},
	},
	{
		[]interface{}{Signature{"ai"}},
		[]byte{2, 'a', 'i', 0},
	},
	{
		[]interface{}{[]int16{42, 256}},
		[]byte{0, 0, 0, 4, 0, 42, 1, 0},
	},
	{
		[]interface{}{int16(42), int32(42)},
		[]byte{0, 42, 0, 0, 0, 0, 0, 42},
	},
	{
		[]interface{}{MakeVariant("foo")},
		[]byte{1, 's', 0, 0, 0, 0, 0, 3, 'f', 'o', 'o', 0},
	},
	{
		[]interface{}{MakeVariant(MakeVariant(Signature{"v"}))},
		[]byte{1, 'v', 0, 1, 'g', 0, 1, 'v', 0},
	},
	{
		[]interface{}{struct {
			A int32
			B int16
		}{10752, 256}},
		[]byte{0, 0, 42, 0, 1, 0},
	},
}

func TestProto(t *testing.T) {
	for i, v := range protoTests {
		buf := new(bytes.Buffer)
		enc := newEncoder(buf, binary.BigEndian)
		enc.Encode(v.vs...)
		marshalled := buf.Bytes()
		if bytes.Compare(marshalled, v.marshalled) != 0 {
			t.Errorf("test %d (marshal): got '%v', but expected '%v'\n", i+1, marshalled,
				v.marshalled)
		}
		unmarshalled := reflect.MakeSlice(reflect.TypeOf(v.vs),
			0, 0)
		for i := range v.vs {
			unmarshalled = reflect.Append(unmarshalled,
				reflect.New(reflect.TypeOf(v.vs[i])))
		}
		dec := newDecoder(bytes.NewReader(v.marshalled), binary.BigEndian)
		unmarshal := reflect.ValueOf(dec).MethodByName("Decode")
		ret := unmarshal.CallSlice([]reflect.Value{unmarshalled})
		err := ret[0].Interface()
		if err != nil {
			t.Errorf("test %d: %s\n", i+1, err)
			continue
		}
		for j := range v.vs {
			if !reflect.DeepEqual(unmarshalled.Index(j).Elem().Elem().Interface(), v.vs[j]) {
				t.Errorf("test %d (unmarshal): got '%v'/'%T', but expected '%v'/'%T'\n",
					i+1, unmarshalled.Index(j).Elem().Elem().Interface(),
					unmarshalled.Index(j).Elem().Elem().Interface(), v.vs[j], v.vs[j])
			}
		}
	}
}

func TestProtoPointer(t *testing.T) {
	var n uint32
	var p = &n
	buf := bytes.NewBuffer([]byte{42, 1, 0, 0})
	dec := newDecoder(buf, binary.LittleEndian)
	if err := dec.Decode(&p); err != nil {
		t.Fatal(err)
	}
	if n != 298 {
		t.Error("pointer test: got", n)
	}
}

func TestProtoSlice(t *testing.T) {
	b := []byte{1, 2}
	buf := bytes.NewBuffer([]byte{2, 0, 0, 0, 3, 4})
	dec := newDecoder(buf, binary.LittleEndian)
	if err := dec.Decode(&b); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, []byte{1, 2, 3, 4}) {
		t.Fatal("got", b, "wanted", []byte{1, 2, 3, 4})
	}
}

func TestProtoMap(t *testing.T) {
	m := map[string]uint8{
		"foo": 23,
		"bar": 2,
	}
	var n map[string]uint8
	buf := new(bytes.Buffer)
	enc := newEncoder(buf, binary.LittleEndian)
	enc.Encode(m)
	dec := newDecoder(buf, binary.LittleEndian)
	dec.Decode(&n)
	if len(n) != 2 || n["foo"] != 23 || n["bar"] != 2 {
		t.Error("map test: got", n)
	}
}

func TestProtoVariantStruct(t *testing.T) {
	var variant Variant
	v := MakeVariant(struct {
		A int32
		B int16
	}{1, 2})
	buf := new(bytes.Buffer)
	enc := newEncoder(buf, binary.LittleEndian)
	enc.Encode(v)
	dec := newDecoder(buf, binary.LittleEndian)
	dec.Decode(&variant)
	sl := variant.Value().([]interface{})
	v1, v2 := sl[0].(int32), sl[1].(int16)
	if v1 != int32(1) {
		t.Error("variant struct test: got", v1)
	}
	if v2 != int16(2) {
		t.Error("variant struct test: got", v2)
	}
}

func TestProtoStructTag(t *testing.T) {
	type Bar struct {
		A int32
		B chan interface{} `dbus:"-"`
		C int32
	}
	var bar1, bar2 Bar
	bar1.A = 234
	bar2.C = 345
	buf := new(bytes.Buffer)
	enc := newEncoder(buf, binary.LittleEndian)
	enc.Encode(bar1)
	dec := newDecoder(buf, binary.LittleEndian)
	dec.Decode(&bar2)
	if bar1 != bar2 {
		t.Error("struct tag test: got", bar2)
	}
}

func TestProtoStoreStruct(t *testing.T) {
	var foo struct {
		A int32
		B string
		c chan interface{}
		D interface{} `dbus:"-"`
	}
	src := []interface{}{[]interface{}{int32(42), "foo"}}
	err := Store(src, &foo)
	if err != nil {
		t.Fatal(err)
	}
}

func TestProtoStoreNestedStruct(t *testing.T) {
	var foo struct {
		A int32
		B struct {
			C string
			D float64
		}
	}
	src := []interface{}{
		[]interface{}{
			int32(42),
			[]interface{}{
				"foo",
				3.14,
			},
		},
	}
	err := Store(src, &foo)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMessage(t *testing.T) {
	buf := new(bytes.Buffer)
	message := new(Message)
	message.Type = TypeMethodCall
	message.serial = 32
	message.Headers = map[HeaderField]Variant{
		FieldPath:   MakeVariant(ObjectPath("/org/foo/bar")),
		FieldMember: MakeVariant("baz"),
	}
	message.Body = make([]interface{}, 0)
	err := message.EncodeTo(buf, binary.LittleEndian)
	if err != nil {
		t.Error(err)
	}
	_, err = DecodeMessage(buf)
	if err != nil {
		t.Error(err)
	}
}

// ordinary org.freedesktop.DBus.Hello call
var smallMessage = &Message{
	Type:   TypeMethodCall,
	serial: 1,
	Headers: map[HeaderField]Variant{
		FieldDestination: MakeVariant("org.freedesktop.DBus"),
		FieldPath:        MakeVariant(ObjectPath("/org/freedesktop/DBus")),
		FieldInterface:   MakeVariant("org.freedesktop.DBus"),
		FieldMember:      MakeVariant("Hello"),
	},
}

// org.freedesktop.Notifications.Notify
var bigMessage = &Message{
	Type:   TypeMethodCall,
	serial: 2,
	Headers: map[HeaderField]Variant{
		FieldDestination: MakeVariant("org.freedesktop.Notifications"),
		FieldPath:        MakeVariant(ObjectPath("/org/freedesktop/Notifications")),
		FieldInterface:   MakeVariant("org.freedesktop.Notifications"),
		FieldMember:      MakeVariant("Notify"),
		FieldSignature:   MakeVariant(Signature{"susssasa{sv}i"}),
	},
	Body: []interface{}{
		"app_name",
		uint32(0),
		"dialog-information",
		"Notification",
		"This is the body of a notification",
		[]string{"ok", "Ok"},
		map[string]Variant{
			"sound-name": MakeVariant("dialog-information"),
		},
		int32(-1),
	},
}

func BenchmarkDecodeMessageSmall(b *testing.B) {
	var err error
	var rd *bytes.Reader

	b.StopTimer()
	buf := new(bytes.Buffer)
	err = smallMessage.EncodeTo(buf, binary.LittleEndian)
	if err != nil {
		b.Fatal(err)
	}
	decoded := buf.Bytes()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		rd = bytes.NewReader(decoded)
		_, err = DecodeMessage(rd)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeMessageBig(b *testing.B) {
	var err error
	var rd *bytes.Reader

	b.StopTimer()
	buf := new(bytes.Buffer)
	err = bigMessage.EncodeTo(buf, binary.LittleEndian)
	if err != nil {
		b.Fatal(err)
	}
	decoded := buf.Bytes()
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		rd = bytes.NewReader(decoded)
		_, err = DecodeMessage(rd)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeMessageSmall(b *testing.B) {
	var err error
	for i := 0; i < b.N; i++ {
		err = smallMessage.EncodeTo(ioutil.Discard, binary.LittleEndian)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeMessageBig(b *testing.B) {
	var err error
	for i := 0; i < b.N; i++ {
		err = bigMessage.EncodeTo(ioutil.Discard, binary.LittleEndian)
		if err != nil {
			b.Fatal(err)
		}
	}
}
