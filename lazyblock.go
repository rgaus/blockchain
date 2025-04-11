package main

type LazyBlock struct {
	chain         *Blockchain `json:"-"`
	Hash          *BlockHash  `json:"hash"`
	resolvedBlock *Block      `json:"-"`
}

var EMPTY_LAZY_BLOCK = &LazyBlock{Hash: nil, resolvedBlock: nil}

func NewLazyBlock(chain *Blockchain, block *Block) *LazyBlock {
	return &LazyBlock{chain: chain, Hash: block.Hash, resolvedBlock: block}
}
func NewLazyBlockFromHash(chain *Blockchain, hash *BlockHash) *LazyBlock {
	return &LazyBlock{chain: chain, Hash: hash, resolvedBlock: nil}
}
func (l *LazyBlock) Unwrap() *Block {
	if l.resolvedBlock != nil {
		return l.resolvedBlock
	}

	if l.Hash == nil {
		return nil
	}

	l.resolvedBlock = l.chain.GetBlockWithHash(*l.Hash)
	return l.resolvedBlock
}
