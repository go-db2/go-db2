package security

import (
	"bytes"
	"crypto/des"
	"encoding/hex"
	"math/big"
	"testing"
)

func TestDiffieHellmanSECMEC9(t *testing.T) {
	// 1. Generate client private key
	clientPriv, err := GenerateDHPrivateKey()
	if err != nil {
		t.Fatalf("GenerateDHPrivateKey failed: %v", err)
	}

	// 2. Compute client public key
	clientPub := CalculateDHPublicKey(clientPriv)
	if len(clientPub) != 32 {
		t.Fatalf("Expected 32-byte public key, got %d bytes", len(clientPub))
	}

	// 3. Simulate server generating private & public key
	serverPriv, err := GenerateDHPrivateKey()
	if err != nil {
		t.Fatalf("Generate server key failed: %v", err)
	}
	serverPub := CalculateDHPublicKey(serverPriv)

	// 4. Client derives session key from server's public key
	clientSessionKey, err := CalculateSessionKey(serverPub, clientPriv)
	if err != nil {
		t.Fatalf("Client CalculateSessionKey failed: %v", err)
	}

	// 5. Server derives session key from client's public key
	serverSessionKey, err := CalculateSessionKey(clientPub, serverPriv)
	if err != nil {
		t.Fatalf("Server CalculateSessionKey failed: %v", err)
	}

	// 6. Both derived session keys must be strictly equal
	if !bytes.Equal(clientSessionKey, serverSessionKey) {
		t.Fatalf("Session keys mismatch!\nClient: %x\nServer: %x", clientSessionKey, serverSessionKey)
	}
}

func TestEncryptPasswordSECMEC9(t *testing.T) {
	// Fixed test vector
	serverSecTknHex := "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	serverSecTkn, _ := hex.DecodeString(serverSecTknHex)

	clientPriv := big.NewInt(123456789)
	password := "SecretPass123"

	encrypted, err := EncryptPasswordSECMEC9(password, serverSecTkn, clientPriv)
	if err != nil {
		t.Fatalf("EncryptPasswordSECMEC9 failed: %v", err)
	}

	if len(encrypted)%des.BlockSize != 0 {
		t.Fatalf("Encrypted length %d is not a multiple of DES block size", len(encrypted))
	}

	// Verify decryption using same derived key
	sessionKey, _ := CalculateSessionKey(serverSecTkn, clientPriv)
	iv := serverSecTkn[12:20]
	key := sessionKey[12:20]

	block, err := des.NewCipher(key)
	if err != nil {
		t.Fatalf("des.NewCipher failed: %v", err)
	}

	decrypted := make([]byte, len(encrypted))
	for i := 0; i < len(encrypted); i += des.BlockSize {
		block.Decrypt(decrypted[i:i+des.BlockSize], encrypted[i:i+des.BlockSize])
	}
	// XOR with IV for first block (CBC mode)
	for j := 0; j < des.BlockSize; j++ {
		decrypted[j] ^= iv[j]
	}
	for i := des.BlockSize; i < len(encrypted); i += des.BlockSize {
		for j := 0; j < des.BlockSize; j++ {
			decrypted[i+j] ^= encrypted[i-des.BlockSize+j]
		}
	}

	// Remove PKCS#5 padding
	padLen := int(decrypted[len(decrypted)-1])
	if padLen > 0 && padLen <= des.BlockSize {
		decrypted = decrypted[:len(decrypted)-padLen]
	}

	if string(decrypted) != password {
		t.Fatalf("Decrypted password mismatch! Got: '%s', expected: '%s'", string(decrypted), password)
	}
}
