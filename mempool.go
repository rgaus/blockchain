package main

import (
  "encoding/json"
)

type MemPool struct {
  Transactions []*Transaction
}
func NewMemPool() *MemPool {
  return &MemPool{
    Transactions: []*Transaction{},
  }
}
func (m MemPool) MarshalJSON() ([]byte, error) {
  var serializedTransactions = []string{}
  for _, t := range m.Transactions {
    serializedBytes, err := t.Serialize()
    if err != nil {
      return nil, err
    }
    serializedTransactions = append(serializedTransactions, string(serializedBytes))
  }

  return json.Marshal(map[string]interface{}{
    "transactions": serializedTransactions,
  })
}
func (m *MemPool) Submit(txn *Transaction) bool {
  for _, t := range m.Transactions {
    if t.Id == txn.Id {
      return false
    }
  }

  m.Transactions = append(m.Transactions, txn)
  return true
}
func (m *MemPool) Clear() {
  m.Transactions = []*Transaction{}
}
