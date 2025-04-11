package main

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/render"
	"github.com/google/uuid"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

func addPeerInRequest(peerSet *PeerSet, r *http.Request) {
	rawPeerInfo, ok := r.Header["X-Peer-Info"]

	if !ok {
		return
	}

	peerInfo := strings.Split(strings.Join(rawPeerInfo, " "), " ")

	if len(peerInfo) < 2 {
		fmt.Printf("Warning: X-Peer-Info header contains less than 2 parts! (%+v)\n", peerInfo)
		return
	}

	peerId, err := uuid.Parse(peerInfo[0])
	if err != nil {
		fmt.Printf("Warning: X-Peer-Info header is invalid: %s\n", err)
		return
	}

	if peerSet.Has(PeerId(peerId)) {
		return
	}

	if err := peerSet.InsertByAddress(peerInfo[1]); err != nil {
		fmt.Printf("Warning: failed to get peer info: %s", err)
	}
}

func sendBlockBytesToPeers(peerSet *PeerSet, blockBytes []byte) {
	for _, peer := range peerSet.ListOthers() {
		resp, err := http.Post(
			fmt.Sprintf("%s/v1/blocks", peer.Address),
			"text/plain",
			bytes.NewBuffer(blockBytes),
		)
		if err != nil {
			fmt.Printf("Failed to propegate block to peer %s! %s\n", uuid.UUID(peer.Id).String(), err)
			peerSet.Decrement(peer.Id, NODE_PEER_OFFLINE_DECREMENT)
			continue
		}
		if resp.StatusCode != 200 {
			fmt.Printf("Failed to propegate block to peer %s, failed with %d!\n", peer, resp.StatusCode)
			peerSet.Decrement(peer.Id, NODE_PEER_INVALID_REQUEST_DECREMENT)
			continue
		}
	}
}

func setupNode(args []string) {
	nodeCmd := flag.NewFlagSet("node", flag.ExitOnError)

	peersRaw := nodeCmd.String("peers", "", "Comma-seperated list of peers to propegate network events to")
	addressRaw := nodeCmd.String("address", "", "Network address other peers can use to reach this peer")

	if err := nodeCmd.Parse(args); err != nil {
		panic(err)
	}

	if len(*addressRaw) == 0 {
		panic("--address is required!")
	}
	peerSet := NewPeerSet(*addressRaw)
	memPool := NewMemPool()
	chain := NewBlockchain()

	r := chi.NewRouter()
	r.Use(middleware.Logger)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Ryan made a blockchain.")
	})

	r.Post("/v1/blocks", func(w http.ResponseWriter, r *http.Request) {
		byt, err := ioutil.ReadAll(r.Body)
		if err != nil {
			render.JSON(w, r, map[string]interface{}{"error": "Error readng body!"})
			return
		}

		newBlock, err := NewBlockFromBytes(chain, byt)
		if err != nil {
			render.JSON(w, r, map[string]interface{}{"error": "Error parsing body into block!"})
			return
		}

		ok, err := newBlock.Verify()
		if err != nil {
			render.JSON(w, r, map[string]interface{}{"error": "Error verifying block"})
			return
		}

		if !ok {
			render.JSON(w, r, map[string]interface{}{"error": "Block could not be validated, rejecting."})
			return
		}

		// If the block is valid, further propegate it
		if ok := chain.InsertBlockAndPlaceIntoAppendage(newBlock); ok {
			sendBlockBytesToPeers(peerSet, byt)
		}
		render.JSON(w, r, map[string]interface{}{"status": "ok"})
	})

	r.Get("/v1/chain", func(w http.ResponseWriter, r *http.Request) {
		render.JSON(w, r, chain)
	})

	r.Get("/v1/blocks/{hash}", func(w http.ResponseWriter, r *http.Request) {
		hash, err := HexToBlockHash(chi.URLParam(r, "hash"))
		if err != nil {
			render.JSON(w, r, map[string]interface{}{"error": "Error parsing block hash!"})
			return
		}
		if hash == nil {
			render.JSON(w, r, map[string]interface{}{"error": "Hash is nil!"})
		}
		block := chain.GetBlockWithHash(*hash)
		if block == nil {
			render.JSON(w, r, map[string]interface{}{"error": "Block not found!"})
		} else {
			byt, err := block.Serialize()
			if err != nil {
				render.JSON(w, r, map[string]interface{}{"error": "Failed to serialize block!"})
				return
			}
			render.JSON(w, r, map[string]interface{}{"block": string(byt)})
		}
	})

	r.Get("/v1/mempool", func(w http.ResponseWriter, r *http.Request) {
		render.JSON(w, r, memPool)
	})

	r.Get("/v1/me", func(w http.ResponseWriter, r *http.Request) {
		render.JSON(w, r, peerSet.Me)
	})

	r.Get("/v1/peers", func(w http.ResponseWriter, r *http.Request) {
		// Add this peer if we've never heard of them before
		addPeerInRequest(peerSet, r)

		render.JSON(w, r, map[string]interface{}{"peers": peerSet.List()})
	})

	// Submit a transaction, eiher from a client or a peer
	r.Post("/v1/transactions", func(w http.ResponseWriter, r *http.Request) {
		byt, err := ioutil.ReadAll(r.Body)
		if err != nil {
			render.JSON(w, r, map[string]interface{}{"error": "Error readng body!"})
			return
		}
		transaction, err := NewTransactionFromBytes(byt)
		if err != nil {
			render.JSON(w, r, map[string]interface{}{"error": "Error parsing transaction!"})
			return
		}

		if ok := memPool.Submit(transaction); ok {
			// If the transaction was newly added to the mempool, proegate it to other nodes
			for _, peer := range peerSet.ListOthers() {
				resp, err := http.Post(
					fmt.Sprintf("%s/v1/transactions", peer.Address),
					"text/plain",
					bytes.NewBuffer(byt),
				)
				if err != nil {
					fmt.Printf("Failed to propegate transaction to peer %s! %s\n", uuid.UUID(peer.Id).String(), err)
					peerSet.Decrement(peer.Id, NODE_PEER_OFFLINE_DECREMENT)
					continue
				}
				if resp.StatusCode != 200 {
					fmt.Printf("Failed to propegate transaction to peer %s, failed with %d!\n", peer, resp.StatusCode)
					peerSet.Decrement(peer.Id, NODE_PEER_INVALID_REQUEST_DECREMENT)
					continue
				}
			}
		}
	})

	var wg sync.WaitGroup
	wg.Add(3)

	// HTTP SERVER
	go func() {
		defer wg.Done()

		port := ":3000"
		if rawPort, ok := os.LookupEnv("PORT"); ok {
			port = fmt.Sprintf(":%s", rawPort)
		}
		fmt.Printf("Running on %s\n", port)

		http.ListenAndServe(port, r)
	}()

	// MANAGE PEERS
	go func() {
		defer wg.Done()

		if len(*peersRaw) > 0 {
			// If there are peers... connect to them!
			fmt.Println("Setting up peerset...")
			peers := strings.Split(*peersRaw, ",")
			for _, rawPeerAddress := range peers {
				peerAddress := strings.Trim(rawPeerAddress, " ")
				if err := peerSet.InsertByAddress(peerAddress); err != nil {
					panic(err)
				}
			}
			peerSet.Refresh()
			fmt.Printf("Peerset configured, %d valid peer(s) found\n", peerSet.Count())
		} else {
			fmt.Println("No valid peers found.")
		}

		if peerSet.Count() > 1 {
			// TODO: Ask the one we trust the most to give us a chain!
			// FIXME: This is a pretty import operation and could be the source of DOS attacks
			highestTrustedPeer := peerSet.ListOthers()[0]
			// FIXME: there must be a better way to make sure we aren't talking to ourself
			if highestTrustedPeer.Address == peerSet.Me.Address {
				highestTrustedPeer = peerSet.ListOthers()[1]
			}
			fmt.Printf("Begin syncing chain from peer %s\n", uuid.UUID(highestTrustedPeer.Id).String())
			resp, err := http.Get(fmt.Sprintf("%s/v1/chain", highestTrustedPeer.Address))
			if err != nil {
				panic(fmt.Sprintf("Failed to get chain from peer with address %s! %s\n", highestTrustedPeer.Address, err))
			}
			if resp.StatusCode != 200 {
				panic(fmt.Sprintf("Failed to get chain from peer with address %s, failed with %d!\n", highestTrustedPeer.Address, resp.StatusCode))
			}
			fmt.Printf("Got chain data from peer %s\n", uuid.UUID(highestTrustedPeer.Id).String())

			defer resp.Body.Close()
			body, err2 := ioutil.ReadAll(resp.Body)
			if err2 != nil {
				panic(fmt.Sprintf("Failed to parse body when getting chain from peer with address %s! %s\n", highestTrustedPeer.Address, err2))
			}
			var response struct {
				Appendages []*struct {
					EncodedGenesis string    `json:"genesis"`
					EncodedHead    string    `json:"head"`
					Length         uint      `json:"chain_length"`
					UpdatedAt      time.Time `json:"updated_at"`
				} `json:"appendages"`
			}
			err = json.Unmarshal(body, &response)
			if err != nil {
				panic(fmt.Sprintf("Failed to parse json body when getting chain from peer with address %s! %s\n", highestTrustedPeer.Address, err))
			}

			fmt.Printf("Fetching data from %d appendage(s)...\n", len(response.Appendages))
			for _, appendage := range response.Appendages {
				headBlock, err := NewBlockFromBytes(chain, []byte(appendage.EncodedHead))
				if err != nil {
					panic(fmt.Sprintf("Failed to parse head block when getting chain from peer with address %s! %s\n", highestTrustedPeer.Address, err))
				}

				genesisBlock, err2 := NewBlockFromBytes(chain, []byte(appendage.EncodedGenesis))
				if err2 != nil {
					panic(fmt.Sprintf("Failed to parse head block when getting chain from peer with address %s! %s\n", highestTrustedPeer.Address, err2))
				}

				chainLength := uint(0)

				chain.InsertBlock(headBlock)
				currentBlock := headBlock
				chainLength += 1
				fmt.Printf("Added head block %x in appendage\n", *headBlock.Hash)

				// fmt.Printf("GENESIS=%+v HEAD=%+v\n", genesisBlock, headBlock)

				// Starting at the head, trace the chain all the way back to the genesis
				for fmt.Sprintf("%x", *currentBlock.Hash) != fmt.Sprintf("%x", *genesisBlock.Hash) {
					chainLength += 1
					previousHash := currentBlock.Previous.Hash
					fmt.Printf("Fetching block %x...\n", *previousHash)

					// fetch previous block
					resp, err := http.Get(fmt.Sprintf("%s/v1/blocks/%x", highestTrustedPeer.Address, *previousHash))
					if err != nil {
						panic(fmt.Sprintf("Failed to get block %x from peer with address %s! %s\n", previousHash, highestTrustedPeer.Address, err))
					}
					if resp.StatusCode != 200 {
						panic(fmt.Sprintf("Failed to get block %x from peer with address %s, failed with %d!\n", previousHash, highestTrustedPeer.Address, resp.StatusCode))
					}

					defer resp.Body.Close()
					body, err2 := ioutil.ReadAll(resp.Body)
					if err2 != nil {
						panic(fmt.Sprintf("Failed to parse body when getting block %x from peer with address %s! %s\n", previousHash, highestTrustedPeer.Address, err2))
					}
					var response struct {
						Block string `json:"block"`
					}
					err = json.Unmarshal(body, &response)
					if err != nil {
						panic(fmt.Sprintf("Failed to parse json body when getting chain from peer with address %s! %s\n", highestTrustedPeer.Address, err))
					}

					previousBlock, err := NewBlockFromBytes(chain, []byte(response.Block))
					if err != nil {
						panic(fmt.Sprintf("Failed to parse block when getting block %x from peer with address %s! %s\n", previousBlock, highestTrustedPeer.Address, err))
					}

					chain.InsertBlock(previousBlock)
					currentBlock = previousBlock
				}

				chain.Appendages = append(chain.Appendages, &BlockchainAppendage{
					Genesis:   genesisBlock,
					Head:      headBlock,
					Length:    chainLength,
					UpdatedAt: appendage.UpdatedAt,
				})

				fmt.Printf("Fetched %d block(s) in appendage\n", chainLength)
			}
		} else {
			// We're on our own... so start our own chain!
			newBlock := NewBlock(nil, []*Transaction{})
			newBlock.Mine()
			chain.InsertBlockAndPlaceIntoAppendage(newBlock)
			fmt.Printf("Created genesis block: %x\n", *newBlock.Hash)
		}

		for {
			time.Sleep(5 * time.Second)
			peerSet.Refresh()
		}
	}()

	// MINING
	go func() {
		defer wg.Done()

		// Later on, a miner does this:
		for {
			time.Sleep(5 * time.Second)

			if len(memPool.Transactions) == 0 {
				continue
			}

			primaryAppendage := chain.PrimaryAppendage()
			if primaryAppendage == nil {
				fmt.Println("There is not a primary appendage, so cannot process new transactions from the mempool!")
				continue
			}

			newBlock := NewBlock(
				NewLazyBlock(chain, primaryAppendage.Head),
				memPool.Transactions,
			)
			newBlock.Mine()
			fmt.Printf("Mined new block: %x\n", newBlock.Hash)

			byt, err := newBlock.Serialize()
			if err != nil {
				fmt.Printf("Cannot serialize block: %s\n", err)
				continue
			}

			// Add block to chain
			chain.InsertBlockAndPlaceIntoAppendage(newBlock)

			// Clear mempool - these transactions are now in the new block!
			memPool.Clear()

			// Prepegate it to others!
			sendBlockBytesToPeers(peerSet, byt)
			// fmt.Sprintf(string(byt))
		}
	}()

	wg.Wait()
}

func submit(args []string) {
	submitCmd := flag.NewFlagSet("submit", flag.ExitOnError)

	addressRaw := submitCmd.String("address", "", "Network address to submit transaction to")
	keyRaw := submitCmd.String("key", "", "File path to rsa private key")
	data := submitCmd.String("data", "", "Data to include in the transaction")

	if err := submitCmd.Parse(args); err != nil {
		panic(err)
	}

	if len(*data) == 0 {
		panic("--data is required!")
	}

	if len(*addressRaw) == 0 {
		panic("--address is required!")
	}

	if len(*keyRaw) == 0 {
		panic("--key is required!")
	}

	privateFile, err1 := ioutil.ReadFile(*keyRaw)
	if err1 != nil {
		panic(err1)
	}

	block, _ := pem.Decode(privateFile)
	if block == nil || block.Type != "RSA PRIVATE KEY" {
		panic("Corrupted key!")
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		panic(err)
	}

	dataBytes := []byte(*data)
	transaction := NewTransaction(privateKey, 0, dataBytes)
	byt, err := transaction.Serialize()
	if err != nil {
		panic(err)
	}

	resp, err := http.Post(
		fmt.Sprintf("%s/v1/transactions", *addressRaw),
		"text/plain",
		bytes.NewBuffer(byt),
	)
	if err != nil {
		panic(err)
	}
	if resp.StatusCode != 200 {
		fmt.Println(resp.StatusCode)
	}
}

func generate(args []string) {
	generateCmd := flag.NewFlagSet("generate", flag.ExitOnError)

	filename := generateCmd.String("filename", "", "Filename prefix to write key into")

	if err := generateCmd.Parse(args); err != nil {
		panic(err)
	}

	if len(*filename) == 0 {
		panic("--filename is required!")
	}

	privateKey, err := NewKeyPair()
	if err != nil {
		panic(err)
	}

	privateFile, err1 := os.OpenFile(*filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err1 != nil {
		panic(err1)
	}

	err = pem.Encode(
		privateFile,
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		},
	)
	if err != nil {
		panic(err)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Missing subcommand!")
		return
	}

	switch os.Args[1] {
	case "node":
		setupNode(os.Args[2:])
	case "submit":
		submit(os.Args[2:])
	case "generate":
		generate(os.Args[2:])
	case "help":
		fmt.Println("This application implements a toy blockchain so that I can learn more about how they work.")
		fmt.Println("For more info on the whole system and how it works, see https://github.com/rgaus/blockchain")
		fmt.Println()
		fmt.Println("Subcommands:")
		fmt.Println("- node")
		fmt.Println("- submit")
		fmt.Println("- generate")
		fmt.Println()
		fmt.Printf("For help on any of the subcommands, run '%s <subcommand> --help'\n", os.Args[0])
	default:
		fmt.Printf("[ERROR] unknown subcommand '%s', see help for more details.\n", os.Args[1])
	}
}
