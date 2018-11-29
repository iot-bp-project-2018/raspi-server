package commproto

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
)

type DatagramType int

const (
	DatagramTypeMessage DatagramType = 0
	DatagramTypeCommand DatagramType = 1
)

type PayloadEncoding int

const (
	PayloadEncodingBinary PayloadEncoding = 0
	PayloadEncodingJSON   PayloadEncoding = 1
)

func MakeMessageWithDetails(sourceAddress string, passphrase string, key []byte, iv []byte, timestamp int32, encoding PayloadEncoding, payload []byte) ([]byte, error) {
	if len(key) != 16 {
		panic("key has wrong length")
	}

	if len(iv) != 16 {
		panic("iv has wrong length")
	}

	format := makeDatagramFormat(DatagramTypeMessage, 0, encoding)

	var aesBuffer bytes.Buffer
	{ // Write plaintext.
		aesBuffer.Write(format[:])
		writeAddress(&aesBuffer, sourceAddress)
		aesBuffer.WriteByte(byte(timestamp >> 24))
		aesBuffer.WriteByte(byte(timestamp >> 16))
		aesBuffer.WriteByte(byte(timestamp >> 8))
		aesBuffer.WriteByte(byte(timestamp >> 0))
		aesBuffer.Write(payload)
	}

	{ // Add PKCS#7 padding.
		padding := aes.BlockSize - aesBuffer.Len()%aes.BlockSize
		for i := 0; i < padding; i++ {
			aesBuffer.WriteByte(byte(padding))
		}
	}

	{ // Encrypt buffer.
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		mode := cipher.NewCBCEncrypter(block, iv)
		mode.CryptBlocks(aesBuffer.Bytes(), aesBuffer.Bytes()) // encrypt aesBuffer in-place
	}

	aesLength := aesBuffer.Len()
	{ // Check payload length.
		expectedHmacLength := 16 + 2 + aesLength // iv + aes ciphertext size + aes ciphertext
		if expectedHmacLength > math.MaxUint16 {
			return nil, errors.New("payload too long")
		}
	}

	var hmacBuffer bytes.Buffer
	{ // Write HMAC message.
		hmacBuffer.Write(iv)
		hmacBuffer.WriteByte(byte(aesLength >> 8))
		hmacBuffer.WriteByte(byte(aesLength >> 0))
		hmacBuffer.Write(aesBuffer.Bytes())
	}

	hmacLength := hmacBuffer.Len()

	var mac []byte
	{ // Calculate MAC.
		hash := hmac.New(sha256.New, []byte(passphrase))
		hash.Write(hmacBuffer.Bytes())
		mac = hash.Sum(nil)
	}

	var datagramBuffer bytes.Buffer
	{ // Write final datagram.
		datagramBuffer.Write(format[:])
		writeAddress(&datagramBuffer, sourceAddress)
		datagramBuffer.WriteByte(byte(hmacLength >> 8))
		datagramBuffer.WriteByte(byte(hmacLength >> 0))
		datagramBuffer.Write(hmacBuffer.Bytes())
		datagramBuffer.Write(mac)
	}

	return datagramBuffer.Bytes(), nil
}

func writeAddress(buffer *bytes.Buffer, address string) {
	length := len(address)
	if length > math.MaxUint8 {
		panic("address too long")
	}
	buffer.WriteByte(byte(length))
	buffer.WriteString(address)
}

func makeDatagramFormat(datagramType DatagramType, version int, encoding PayloadEncoding) (format [3]byte) {
	switch datagramType {
	case DatagramTypeMessage:
		format[0] = 'M'
	case DatagramTypeCommand:
		format[0] = 'C'
	default:
		panic(fmt.Sprintf("invalid datagram type: %d", datagramType))
	}

	if version < 0 {
		panic(fmt.Sprintf("invalid datagram version: %d", version))
	}
	if version >= 10 {
		panic(fmt.Sprintf("datagram version too big to encode using current format: %d", version))
	}

	format[1] = byte('0' + version)

	switch encoding {
	case PayloadEncodingBinary:
		format[2] = 'B'
	case PayloadEncodingJSON:
		format[2] = 'J'
	default:
		panic(fmt.Sprintf("invalid payload encoding: %d", encoding))
	}

	return
}

// Extracts all information from the unencrypted parts of a datagram buffer.
func GetDatagramPublicInformation(datagram []byte) (datagramType DatagramType, version int, encoding PayloadEncoding, sourceAddress string, err error) {
	if len(datagram) < 4 {
		err = errors.New("datagram too short")
		return
	}

	switch datagram[0] {
	case 'M':
		datagramType = DatagramTypeMessage
	case 'C':
		datagramType = DatagramTypeCommand
	default:
		err = errors.New("unknown datagram type")
		return
	}

	switch datagram[1] {
	case '0':
		version = 0
	default:
		err = errors.New("unknown version")
		return
	}

	switch datagram[2] {
	case 'B':
		encoding = PayloadEncodingBinary
	case 'J':
		encoding = PayloadEncodingJSON
	default:
		err = errors.New("unknown payload encoding")
		return
	}

	length := int(datagram[3])
	if len(datagram) < 4+length {
		err = errors.New("invalid address length")
		return
	}

	sourceAddress = string(datagram[4 : 4+length])
	return
}

// @Todo: Assumes well-formed data right now!!!
func DecodeMessageWithDetails(datagram []byte, passphrase string, key []byte) (timestamp int32, payload []byte, err error) {
	if len(key) != 16 {
		panic("key has wrong length")
	}

	publicLength := 4 + int(datagram[3])

	hmacLengthHigh := int(datagram[publicLength])
	hmacLengthLow := int(datagram[publicLength+1])
	hmacLength := (hmacLengthHigh << 8) + hmacLengthLow
	hmacStart := publicLength + 2

	var expectedMAC []byte
	{ // Calculate MAC.
		hash := hmac.New(sha256.New, []byte(passphrase))
		hash.Write(datagram[hmacStart : hmacStart+hmacLength])
		expectedMAC = hash.Sum(nil)
	}

	messageMAC := datagram[hmacStart+hmacLength : hmacStart+hmacLength+sha256.Size]

	if len(datagram) != hmacStart+hmacLength+sha256.Size {
		err = errors.New("invalid datagram")
		return
	}

	if !hmac.Equal(messageMAC, expectedMAC) {
		err = errors.New("invalid datagram")
		return
	}

	iv := datagram[hmacStart : hmacStart+16]
	aesLengthHigh := int(datagram[hmacStart+16])
	aesLengthLow := int(datagram[hmacStart+17])
	aesLength := (aesLengthHigh << 8) + aesLengthLow
	aesStart := hmacStart + 16 + 2

	{ // Decrypt data.
		aesBuffer := datagram[aesStart : aesStart+aesLength]
		block, aesErr := aes.NewCipher(key)
		if aesErr != nil {
			err = aesErr
			return
		}
		mode := cipher.NewCBCDecrypter(block, iv)
		mode.CryptBlocks(aesBuffer, aesBuffer) // decrypt in-place
	}

	var padding int
	{ // Check padding.
		padding = int(datagram[aesStart+aesLength-1])
		if padding > aes.BlockSize {
			err = errors.New("invalid datagram")
			return
		}
		for i := 0; i < padding; i++ {
			if int(datagram[aesStart+aesLength-i-1]) != padding {
				err = errors.New("invalid datagram")
				return
			}
		}
	}

	if !bytes.Equal(datagram[0:publicLength], datagram[aesStart:aesStart+publicLength]) {
		err = errors.New("invalid datagram")
		return
	}

	if aesStart+aesLength != hmacStart+hmacLength {
		err = errors.New("invalid datagram")
		return
	}

	timestamp += int32(datagram[aesStart+publicLength+0]) << 24
	timestamp += int32(datagram[aesStart+publicLength+1]) << 16
	timestamp += int32(datagram[aesStart+publicLength+2]) << 8
	timestamp += int32(datagram[aesStart+publicLength+3]) << 0

	payload = datagram[aesStart+publicLength+4 : aesStart+aesLength-padding]
	return
}
