package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
)

func NewKeyPair() (*rsa.PrivateKey, error) {
	return rsa.GenerateKey(rand.Reader, 2048)
}

// Make a new publickey type so that it will marshall into json nicely
type PublicKey rsa.PublicKey

func (p PublicKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"n": p.N.String(),
		"e": p.E,
	})
}
func (p *PublicKey) UnmarshalJSON(byt []byte) error {
	var temp struct {
		N string `json:"n"`
		E int    `json:"e"`
	}
	err := json.Unmarshal(byt, &temp)
	if err != nil {
		return err
	}

	n := new(big.Int)
	n, ok := n.SetString(temp.N, 10)
	if !ok {
		return errors.New(fmt.Sprintf("Cannot convert %s to bigint!", temp.N))
	}
	p.N = n
	p.E = temp.E
	return nil
}
