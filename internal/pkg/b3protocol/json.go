package b3protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"unicode/utf8"
)

var invalidWireJSON = errors.New("invalid B3 JSON")

func checkJSON(raw []byte) error {
	if !utf8.Valid(raw) {
		return invalidWireJSON
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		word, ok := unicodeWord(raw[i:])
		if !ok {
			i++
			continue
		}
		i += 5
		if word >= 0xdc00 && word <= 0xdfff {
			return invalidWireJSON
		}
		if word >= 0xd800 && word <= 0xdbff {
			low, ok := unicodeWord(raw[i+1:])
			if !ok || low < 0xdc00 || low > 0xdfff {
				return invalidWireJSON
			}
			i += 6
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return checkJSONValue(decoder, 0)
}

func unicodeWord(raw []byte) (uint64, bool) {
	if len(raw) < 6 || raw[0] != '\\' || raw[1] != 'u' {
		return 0, false
	}
	n, err := strconv.ParseUint(string(raw[2:6]), 16, 16)
	return n, err == nil
}

func checkJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 12 {
		return invalidWireJSON
	}
	value, err := decoder.Token()
	if err != nil {
		return err
	}
	switch value {
	case json.Delim('{'):
		names := make(map[string]bool)
		for decoder.More() {
			name, err := decoder.Token()
			field, ok := name.(string)
			if err != nil || !ok || !fieldName.MatchString(field) || names[field] {
				return invalidWireJSON
			}
			names[field] = true
			if err := checkJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	case json.Delim('['):
		for decoder.More() {
			if err := checkJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	}
	return err
}
