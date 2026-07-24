package server

import "encoding/base64"

const qoderCustomAlphabet = "_doRTgHZBKcGVjlvpC,@aFSx#DPuNJme&i*MzLOEn)sUrthbf%Y^w.(kIQyXqWA!"

func qoderEncodeBody(plain []byte) []byte {
	encoded := base64.StdEncoding.EncodeToString(plain)
	third := len(encoded) / 3
	rearranged := encoded[len(encoded)-third:] + encoded[third:len(encoded)-third] + encoded[:third]
	result := make([]byte, len(rearranged))
	for index := range rearranged {
		character := rearranged[index]
		if character == '=' {
			result[index] = '$'
			continue
		}
		if position := stringsIndexByte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/", character); position >= 0 {
			result[index] = qoderCustomAlphabet[position]
		} else {
			result[index] = character
		}
	}
	return result
}

func stringsIndexByte(value string, target byte) int {
	for index := 0; index < len(value); index++ {
		if value[index] == target {
			return index
		}
	}
	return -1
}
