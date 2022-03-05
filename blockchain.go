package main
import (
  "fmt"
  "time"
  "encoding/json"
)

type BlockchainAppendage struct {
  Genesis *Block
  Head *Block
  Length uint
  UpdatedAt time.Time
}
func (ba BlockchainAppendage) MarshalJSON() ([]byte, error) {
  genesisBytes, err := ba.Genesis.Serialize()
  if err != nil {
    return nil, err
  }

  headBytes, err := ba.Head.Serialize()
  if err != nil {
    return nil, err
  }

  return json.Marshal(map[string]interface{}{
    "genesis": string(genesisBytes),
    "head": string(headBytes),
    "chain_length": ba.Length,
    "updated_at": ba.UpdatedAt,
  })
}


const BLOCKCHAIN_BTREE_DEGREE = 2;
type Blockchain struct {
  Appendages []*BlockchainAppendage `json:"appendages"`
  index map[BlockHash]*Block
}
func NewBlockchain() *Blockchain {
  return &Blockchain{
    Appendages: []*BlockchainAppendage{},
    index: map[BlockHash]*Block{},
    // btree.New(func(a interface{}, b interface{}) bool {
    //   return fmt.Sprintf("%x", a.(Block).Hash) < fmt.Sprintf("%x", b.(Block).Hash)
    // }),
  }
}
func (c *Blockchain) InsertBlockAndPlaceIntoAppendage(block *Block) bool {
  if ok := c.InsertBlock(block); !ok {
    return false
  }

  // After adding the block to the index, now we need to figure out how it fits into the broader
  // chain.

  // fmt.Printf("Adding block %x\n", block.Hash)
  // fmt.Printf("Previous Block hash %+v\n", block.Previous)
  if block.Previous != nil {
    blockPrevious := block.Previous.Unwrap()
    // fmt.Printf("Unwrapped Previous Block hash %+v\n", blockPrevious)
    if blockPrevious != nil {
      // Search through each appendage to find the one this block goes on top of
      for _, appendage := range c.Appendages {
        if appendage.Head == nil {
          continue
        }
        if appendage.Head.Hash == blockPrevious.Hash {
          fmt.Printf("Existing appendage will fit block %x\n", block.Hash)
          appendage.Head = block
          appendage.Length += 1
          appendage.UpdatedAt = time.Now().UTC()
          return true
        }
      }
      // If no appendage can be found, start decending down each appendage
      // FIXME: this should be a bredth first search, not depth first
      fmt.Printf("No existing appendages will fit block %x on the end\n", block.Hash)
      for index, appendage := range c.Appendages {
        if appendage.Head == nil { continue }
        if appendage.Head.Previous == nil { continue }

        depth := uint(0)
        currentBlock := appendage.Head.Previous.Unwrap()
        for currentBlock != nil {
          fmt.Printf("Search block %d in appendage %d\n", depth, index)
          if currentBlock.Hash == blockPrevious.Hash {
            fmt.Printf("Hit! Making new appendage with common base as %d\n", index)
            c.Appendages = append(c.Appendages, &BlockchainAppendage{
              Genesis: appendage.Genesis,
              Head: block,
              Length: appendage.Length - depth,
              UpdatedAt: time.Now().UTC(),
            })
            return true
          }

          if currentBlock.Previous == nil { break }
          currentBlock = currentBlock.Previous.Unwrap()
          depth += 1
        }
      }
    }
  }

  // If a matching appendage can't be found... make a new appendage!
  c.Appendages = append(c.Appendages, &BlockchainAppendage{
    Genesis: block,
    Head: block,
    Length: 1,
    UpdatedAt: time.Now().UTC(),
  })
  return true
}
func (c *Blockchain) InsertBlock(block *Block) bool {
  if block.Hash == nil {
    return false
  }
  if _, ok := c.index[*block.Hash]; ok {
    return false
  }
  c.index[*block.Hash] = block
  return true
}
func (c *Blockchain) GetBlockWithHash(hash BlockHash) *Block {
  block, ok := c.index[hash]
  if ok {
    return block
  } else {
    return nil
  }
}
func (c *Blockchain) longestAppendageLength() uint {
  length := uint(0)
  for _, appendage := range c.Appendages {
    if appendage.Length > length {
      length = appendage.Length
    }
  }
  return length
}
func (c *Blockchain) LongestAppendages() []*BlockchainAppendage {
  length := c.longestAppendageLength()

  var matchingAppendages []*BlockchainAppendage
  for _, appendage := range c.Appendages {
    if appendage.Length == length {
      matchingAppendages = append(matchingAppendages, appendage)
    }
  }

  return matchingAppendages
}
func (c *Blockchain) PrimaryAppendage() *BlockchainAppendage {
  // Primarily filter based on the appendage that is longest
  appendages := c.LongestAppendages()
  if len(appendages) == 0 {
    return nil
  }
  if len(appendages) == 1 {
    return appendages[0]
  }

  // To deconflict when there are appendages that are the same length, use the newest one
  timestamp := time.Unix(0, 0)
  for _, appendage := range appendages {
    if appendage.UpdatedAt.After(timestamp) {
      timestamp = appendage.UpdatedAt
    }
  }

  var matchingAppendages []*BlockchainAppendage
  for _, appendage := range appendages {
    if appendage.UpdatedAt == timestamp {
      matchingAppendages = append(matchingAppendages, appendage)
    }
  }

  if len(matchingAppendages) == 0 {
    return nil
  }
  if len(matchingAppendages) == 1 {
    return matchingAppendages[0]
  }

  // Finally worst case, just pick the first one
  return matchingAppendages[0]
}
func (c *Blockchain) CullAppendagesShorterThan(minimumLength uint) {
  // This is a goofy way in golang to modify a slice in place
  // ref: https://zetcode.com/golang/filter-slice/
  index := 0
  for _, appendage := range c.Appendages {
    if appendage.Length > minimumLength {
      c.Appendages[index] = appendage
      index += 1
    }
  }
  c.Appendages = c.Appendages[:index]
}
