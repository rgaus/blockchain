package main
import (
  "fmt"
  "errors"
  "strings"
  "encoding/hex"
  "encoding/json"
  "encoding/base64"
  "crypto"
  "crypto/sha256"
  "crypto/rsa"
  "crypto/rand"
  "github.com/google/uuid"
)


type Transaction struct {
  Id uuid.UUID `json:"id"`
  Signature []byte `json:"-"`
  SenderPrivateKey *rsa.PrivateKey `json:"-"`
  SenderPublicKey *PublicKey `json:"public_key"`
  Cost Currency `json:"cost"`

  Data []byte `json:"data"`
}
func NewTransaction(
  sender *rsa.PrivateKey,
  cost Currency,
  data []byte,
) *Transaction {
  pubKey := PublicKey(sender.PublicKey)
  return &Transaction{
    Id: uuid.New(),
    SenderPrivateKey: sender,
    SenderPublicKey: &pubKey,
    Cost: cost,
    Data: data,
    Signature: nil,
  }
}
func NewTransactionFromBytes(bytes []byte) (*Transaction, error) {
  sections := strings.Split(string(bytes), ".")
  if len(sections) != 2 {
    return nil, errors.New("Malformed signature wrapper on transaction!")
  }
  payloadBytes, err0 := base64.StdEncoding.DecodeString(sections[0])
  if err0 != nil {
    return nil, err0
  }
  payload := []byte(payloadBytes)
  signature, err1 := hex.DecodeString(sections[1])
  if err1 != nil {
    return nil, err1
  }

  var transaction Transaction
  err2 := json.Unmarshal(payload, &transaction)
  if err2 != nil {
    return nil, err2
  }

  transaction.Signature = signature

  return &transaction, nil
}
func (t *Transaction) SerializePayload() ([]byte, error) {
  result, err := json.Marshal(t)
  if err != nil {
    return nil, err
  }
  return []byte(base64.StdEncoding.EncodeToString(result)), nil
}

func (t *Transaction) Sign() error {
  if t.SenderPrivateKey == nil {
    return errors.New("Cannot sign transaction without a private key!")
  }
  payload, err1 := t.SerializePayload()
  if err1 != nil {
    return err1
  }
  hashedPayload := sha256.Sum256(payload)
  signature, err2 := rsa.SignPKCS1v15(rand.Reader, t.SenderPrivateKey, crypto.SHA256, hashedPayload[:])
  if err2 != nil {
    return err2
  }
  t.Signature = signature
  return nil
}

func (t *Transaction) Serialize() ([]byte, error) {
  if t.Signature == nil {
    err := t.Sign()
    if err != nil {
      return nil, err
    }
  }
  payload, err := t.SerializePayload()
  if err != nil {
    return nil, err
  }
  return []byte(fmt.Sprintf("%s.%x", payload, t.Signature)), nil
}

func (t *Transaction) Verify() (bool, error) {
  payload, err1 := t.SerializePayload()
  if err1 != nil {
    return false, err1
  }
  hashedPayload := sha256.Sum256(payload)

  senderPublicKeyAsRsa := (*rsa.PublicKey)(t.SenderPublicKey)
  err2 := rsa.VerifyPKCS1v15(senderPublicKeyAsRsa, crypto.SHA256, hashedPayload[:], t.Signature)
  if err2 != nil {
    return false, err2
  }
  return true, nil
}
