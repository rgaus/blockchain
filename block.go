package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Currency uint
type BlockHash [32]byte

const HASH_ZERO_PREFIX_LENGTH = 4

func TestHash(hash BlockHash) bool {
	hashHex := fmt.Sprintf("%x", hash)
	for i := 0; i < HASH_ZERO_PREFIX_LENGTH; i += 1 {
		if hashHex[i] != '0' {
			return false
		}
	}
	return true
}

func HexToBlockHash(hexHash string) (*BlockHash, error) {
	rawHash, err := hex.DecodeString(hexHash)
	if err != nil {
		return nil, err
	}

	var rawHashCopy BlockHash
	for index, byt := range rawHash {
		rawHashCopy[index] = byt
	}
	return &rawHashCopy, nil
}

type Block struct {
	CreatedAt time.Time      `json:"created_at"`
	Previous  *LazyBlock     `json:"previous_block"`
	Data      []*Transaction `json:"data"`
	Number    uint           `json:"number"`
	Hash      *BlockHash     `json:"hash"`
}

func NewBlock(previous *LazyBlock, data []*Transaction) *Block {
	return &Block{
		CreatedAt: time.Now().UTC(),
		Previous:  previous,
		Data:      data,
		Number:    0,
		Hash:      nil,
	}
}
func NewBlockFromBytes(chain *Blockchain, bytes []byte) (*Block, error) {
	sections := strings.Split(string(bytes), ".")
	if len(sections) != 2 {
		return nil, errors.New("Malformed hash wrapper on block!")
	}
	payloadBytes, err0 := base64.StdEncoding.DecodeString(sections[0])
	if err0 != nil {
		return nil, err0
	}
	payload := []byte(payloadBytes)
	hash, err1 := HexToBlockHash(sections[1])
	if err1 != nil {
		return nil, err1
	}

	type BlockRawData struct {
		CreatedAt       time.Time `json:"created_at"`
		PreviousHashHex string    `json:"previous_hash"`
		TransactionsRaw [][]byte  `json:"transactions"`
		Number          uint      `json:"number"`
	}
	var blockRawData BlockRawData
	err2 := json.Unmarshal(payload, &blockRawData)
	if err2 != nil {
		return nil, err2
	}

	previousHash, err3 := HexToBlockHash(blockRawData.PreviousHashHex)
	if err3 != nil {
		return nil, err3
	}

	var transactions []*Transaction
	for _, byt := range blockRawData.TransactionsRaw {
		transaction, err := NewTransactionFromBytes(byt)
		if err != nil {
			return nil, err
		}
		transactions = append(transactions, transaction)
	}

	block := Block{
		CreatedAt: blockRawData.CreatedAt,
		Number:    blockRawData.Number,
		Previous:  NewLazyBlockFromHash(chain, previousHash),
		Data:      transactions,
		Hash:      hash,
	}

	return &block, nil
}
func (b *Block) Serialize() ([]byte, error) {
	if b.Hash == nil {
		return nil, errors.New("Cannot serialize an unmined block!")
	}
	payload, err := b.SerializePayload()
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("%s.%x", payload, *b.Hash)), nil
}
func (b *Block) SerializePayload() ([]byte, error) {
	var serializedTransactions []string = []string{}
	for _, t := range b.Data {
		serializedBytes, err := t.Serialize()
		if err != nil {
			return nil, err
		}
		serializedTransactions = append(serializedTransactions, string(serializedBytes))
	}

	previousHash := ""
	if b.Previous != nil {
		unwrapped := b.Previous.Unwrap()
		if unwrapped != nil {
			previousHash = fmt.Sprintf("%x", *unwrapped.Hash)
		}
	}

	result, err := json.Marshal(map[string]interface{}{
		"created_at":    b.CreatedAt,
		"previous_hash": previousHash,
		"transactions":  serializedTransactions,
		"number":        b.Number,
	})
	if err != nil {
		return nil, err
	}
	return []byte(base64.StdEncoding.EncodeToString(result)), nil
}
func (b *Block) VerifyData() (bool, error) {
	for _, t := range b.Data {
		verified, err := t.Verify()
		if err != nil {
			return false, err
		}
		if !verified {
			return false, nil
		}
	}
	return true, nil
}
func (b *Block) VerifyHash() (*BlockHash, error) {
	serialized, err := b.SerializePayload()
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(serialized)
	if TestHash(hash) {
		a := BlockHash(hash)
		return &a, nil
	} else {
		return nil, nil
	}
}
func (b *Block) Verify() (bool, error) {
	// Make sure transactions signatures are valid
	ok, err := b.VerifyData()
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	// Make sure block hash is valid
	hash, err2 := b.VerifyHash()
	if err2 != nil {
		return false, err2
	}
	if hash == nil {
		return false, nil
	}

	return true, nil
}
func (b *Block) InvalidateHash() {
	b.Hash = nil
}
func (b *Block) Mine() error {
	for n := uint(0); n < MaxUint; n += 1 {
		if n%1000 == 0 {
			fmt.Printf("Mine Status: %d\n", n)
		}
		b.Number = n
		if hash, err := b.VerifyHash(); err == nil && hash != nil {
			b.Hash = hash
			break
		}
	}
	return nil
}
