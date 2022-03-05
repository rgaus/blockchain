package main

import (
  "fmt"
  "errors"
  "sort"
  "net/http"
  "io/ioutil"
  "encoding/json"
  "github.com/google/uuid"
)

type PeerId uuid.UUID
type Peer struct {
  Id PeerId
  Address string
}
func (p Peer) MarshalJSON() ([]byte, error) {
  return json.Marshal(map[string]interface{}{
    "id": uuid.UUID(p.Id).String(),
    "address": p.Address,
  })
}
func (p *Peer) UnmarshalJSON(byt []byte) error {
  var temp struct{
    Id string `json:"id"`
    Address string `json:"address"`
  }
  err := json.Unmarshal(byt, &temp)
  if err != nil {
    return err
  }

  rawPeerId, err := uuid.Parse(temp.Id)
  if err != nil {
    return err
  }
  p.Id = PeerId(rawPeerId)
  p.Address = temp.Address
  return nil
}
func (p *Peer) Header() string {
  return fmt.Sprintf("%s %s", uuid.UUID(p.Id).String(), p.Address)
}
func (p *Peer) Equal(to Peer) bool {
  return p.Id == to.Id && p.Address == to.Address
}
type PeerRanking uint
const NODE_DEFAULT_PEER_RANKING = PeerRanking(10)
const NODE_PEER_OFFLINE_DECREMENT = PeerRanking(1)
const NODE_PEER_INVALID_REQUEST_DECREMENT = PeerRanking(1)
const NODE_PEER_NEW_VALID_PEER_INCREMENT = PeerRanking(2)
const NODE_MINIMUM_PEER_COUNT = 3
const NODE_IDEAL_PEER_COUNT = 10
type PeerSet struct {
  Me Peer
  peers map[PeerId]Peer
  rankings map[PeerId]PeerRanking
  untrusted []PeerId
}
func NewPeerSet(address string) *PeerSet {
  me := Peer{
    Id: PeerId(uuid.New()),
    Address: address,
  }
  return &PeerSet{
    Me: me,
    peers: map[PeerId]Peer{ me.Id: me },
    rankings: map[PeerId]PeerRanking{
      me.Id: NODE_DEFAULT_PEER_RANKING,
    },
    untrusted: []PeerId{},
  }
}
func (ps *PeerSet) Has(id PeerId) bool {
  if _, ok := ps.peers[id]; ok {
    return true
  } else {
    return false
  }
}
func (ps *PeerSet) Untrusted(id PeerId) bool {
  for _, pId := range ps.untrusted {
    if pId == id {
      return true
    }
  }
  return false
}
func (ps *PeerSet) MarkUntrusted(id PeerId) {
  ps.untrusted = append(ps.untrusted, id)
}
func (ps *PeerSet) Count() int {
  return len(ps.rankings)
}
func (ps *PeerSet) Insert(peer Peer) bool {
  // Add peers into the peerset, if they aren't already in the peerset, or already been marked as
  // untrusted
  if ps.Has(peer.Id) {
    return false
  }
  if ps.Untrusted(peer.Id) {
    return false
  }
  if peer.Id == ps.Me.Id {
    return false
  }
  ps.peers[peer.Id] = peer
  ps.rankings[peer.Id] = NODE_DEFAULT_PEER_RANKING
  fmt.Printf("New peer %s (address %s) found!\n", uuid.UUID(peer.Id).String(), peer.Address)
  return true
}
func (ps *PeerSet) InsertByAddress(peerAddress string) error {
  resp, err := http.Get(fmt.Sprintf("%s/v1/me", peerAddress))
  if err != nil {
    return errors.New(fmt.Sprintf("Failed to get info from peer with address %s! %s\n", peerAddress, err))
  }
  if resp.StatusCode != 200 {
    return errors.New(fmt.Sprintf("Failed to get info from peer with address %s, failed with %d!\n", peerAddress, resp.StatusCode))
  }

  defer resp.Body.Close()
  body, err2 := ioutil.ReadAll(resp.Body)
  if err2 != nil {
    return errors.New(fmt.Sprintf("Failed to parse body when getting info from peer with address %s! %s\n", peerAddress, err2))
  }
  var response Peer
  err = json.Unmarshal(body, &response)
  if err != nil {
    return errors.New(fmt.Sprintf("Failed to parse json body when getting info from peer with address %s! %s\n", peerAddress, err))
  }

  ps.Insert(response)
  return nil
}
func (ps *PeerSet) Increment(id PeerId, change PeerRanking) {
  ps.rankings[id] += change
  ps.Rank()
}
func (ps *PeerSet) Decrement(id PeerId, change PeerRanking) {
  if ps.rankings[id] > 0 {
    ps.rankings[id] -= change
  }
  ps.Rank()
}
func (ps *PeerSet) Remove(id PeerId) {
  delete(ps.peers, id)
  delete(ps.rankings, id)
  // But keep it in untrusted! That seems like a good idea
}
func (ps *PeerSet) Rank() {
  // Recompute which peers are trustworthy and untrustworthy
  for k, v := range ps.rankings {
    if v == 0 {
      ps.untrusted = append(ps.untrusted, k)
      delete(ps.rankings, k)
    }
  }
}
func (ps *PeerSet) Refresh() error {
  client := &http.Client{}

  if len(ps.rankings) == 1 {
    return errors.New("This node has no other peers to query for more peers!")
  }

  fmt.Println("Checking to make sure all peers are healthy...")
  for _, peer := range ps.ListOthers() {
    req, err := http.NewRequest("GET", fmt.Sprintf("%s/v1/me", peer.Address), nil)
    if err != nil {
      fmt.Printf("Failed to assemble request for peer %s! %s\n", uuid.UUID(peer.Id).String(), err)
      // Don't decrement the ranking in this case, this is probably not the peer's fault
      continue
    }
    req.Header.Add("X-Peer-Info", ps.Me.Header())
    resp, err1 := client.Do(req)
    if err1 != nil {
      fmt.Printf("Failed to check peer %s health! %s\n", uuid.UUID(peer.Id).String(), err1)
      ps.Decrement(peer.Id, NODE_PEER_OFFLINE_DECREMENT)
      continue
    }
    if resp.StatusCode != 200 {
      fmt.Printf("Failed to check peer %s health, failed with %d!\n", uuid.UUID(peer.Id).String(), resp.StatusCode)
      ps.Decrement(peer.Id, NODE_PEER_INVALID_REQUEST_DECREMENT)
      continue
    }

    defer resp.Body.Close()
    body, err2 := ioutil.ReadAll(resp.Body)
    if err2 != nil {
      fmt.Printf("Failed to parse body when checking peer %s health! %s\n", uuid.UUID(peer.Id).String(), err2)
      ps.Decrement(peer.Id, NODE_PEER_INVALID_REQUEST_DECREMENT)
      continue
    }
    var response Peer
    err = json.Unmarshal(body, &response)
    if err != nil {
      fmt.Printf("Failed to parse json body when checking peer %s health! %s\n", uuid.UUID(peer.Id).String(), err)
      ps.Decrement(peer.Id, NODE_PEER_INVALID_REQUEST_DECREMENT)
      continue
    }

    // Make sure we aren't talking to ourselves!
    if peer.Id == ps.Me.Id {
      ps.Remove(peer.Id);
    }

    if peer.Id != response.Id {
      fmt.Printf("Peer %s now has a different id, removing...\n", uuid.UUID(peer.Id).String())
      ps.Remove(peer.Id)
      ps.MarkUntrusted(peer.Id)
    }
  }
  fmt.Println("Checking to make sure all peers are healthy...done")
  fmt.Printf("Number of healthy peers: %d\n", ps.Count())

  if len(ps.rankings) > NODE_MINIMUM_PEER_COUNT {
    return nil
  }

  // Talk to peers to try to get more peers if we don't have enough
  fmt.Printf("Trying to aquire more peers to get to %d\n", NODE_IDEAL_PEER_COUNT)
  for _, peer := range ps.ListOthers() {
    req, err := http.NewRequest("GET", fmt.Sprintf("%s/v1/peers", peer.Address), nil)
    if err != nil {
      fmt.Printf("Failed to assemble request for peer %s! %s\n", uuid.UUID(peer.Id).String(), err)
      // Don't decrement the ranking in this case, this is probably not the peer's fault
      continue
    }
    req.Header.Add("X-Peer-Info", ps.Me.Header())
    resp, err1 := client.Do(req)
    if err1 != nil {
      fmt.Printf("Failed to get peers from peer %s! %s\n", uuid.UUID(peer.Id).String(), err1)
      ps.Decrement(peer.Id, NODE_PEER_OFFLINE_DECREMENT)
      continue
    }
    if resp.StatusCode != 200 {
      fmt.Printf("Failed to get peers from peer %s, failed with %d!\n", uuid.UUID(peer.Id).String(), resp.StatusCode)
      ps.Decrement(peer.Id, NODE_PEER_INVALID_REQUEST_DECREMENT)
      continue
    }

    defer resp.Body.Close()
    body, err2 := ioutil.ReadAll(resp.Body)
    if err2 != nil {
      fmt.Printf("Failed to parse body when getting peers from peer %s! %s\n", uuid.UUID(peer.Id).String(), err2)
      ps.Decrement(peer.Id, NODE_PEER_INVALID_REQUEST_DECREMENT)
      continue
    }
    var peerResponse struct{ Peers []Peer `json:"peers"` }
    err = json.Unmarshal(body, &peerResponse)
    if err != nil {
      fmt.Printf("Failed to parse json body when getting peers from peer %s! %s\n", uuid.UUID(peer.Id).String(), err)
      ps.Decrement(peer.Id, NODE_PEER_INVALID_REQUEST_DECREMENT)
      continue
    }

    // For each new peer, try to merge it into the existing peer list
    for _, newPeer := range peerResponse.Peers {
      req, err := http.NewRequest("GET", fmt.Sprintf("%s/v1/me", peer.Address), nil)
      if err != nil {
        fmt.Printf("Failed to assemble request for peer %s! %s\n", uuid.UUID(peer.Id).String(), err)
        continue
      }
      req.Header.Add("X-Peer-Info", ps.Me.Header())
      resp, err1 := client.Do(req)
      if err1 != nil {
        fmt.Printf("Failed to check peer %s id! %s\n", uuid.UUID(peer.Id).String(), err1)
        continue
      }
      if resp.StatusCode != 200 {
        fmt.Printf("Failed to check peer %s id, failed with %d!\n", uuid.UUID(peer.Id).String(), resp.StatusCode)
        continue
      }

      defer resp.Body.Close()
      body, err2 := ioutil.ReadAll(resp.Body)
      if err2 != nil {
        fmt.Printf("Failed to parse body when checking peer %s id! %s\n", uuid.UUID(peer.Id).String(), err2)
        ps.Decrement(peer.Id, NODE_PEER_INVALID_REQUEST_DECREMENT)
        continue
      }
      var response Peer
      err = json.Unmarshal(body, &response)
      if err != nil {
        fmt.Printf("Failed to parse json body when checking peer %s id! %s\n", uuid.UUID(peer.Id).String(), err)
        continue
      }

      if uuid.UUID(peer.Id).String() != uuid.UUID(response.Id).String() {
        fmt.Printf("Upon verification, peer %s actually has id %s, rejecting...\n", uuid.UUID(peer.Id).String(), uuid.UUID(response.Id).String())
        continue
      }

      if ok := ps.Insert(newPeer); !ok {
        continue
      }

      ps.Increment(peer.Id, NODE_PEER_NEW_VALID_PEER_INCREMENT)
      fmt.Printf("Successfully added peer %s (from %s)\n", uuid.UUID(newPeer.Id).String(), uuid.UUID(peer.Id).String())

      // Once we have enough peers, then we're done!
      if ps.Count() >= NODE_IDEAL_PEER_COUNT  {
        fmt.Println("Reached ideal peer count!")
        ps.Rank()
        return nil
      }
    }
  }
  fmt.Printf("Number of peers: %d\n", ps.Count())

  ps.Rank()
  return nil
}
// ref: https://medium.com/@kdnotes/how-to-sort-golang-maps-by-value-and-key-eedc1199d944
type Pair struct {
  Key   Peer
  Value PeerRanking
}
type PairList []Pair
func (p PairList) Len() int           { return len(p) }
func (p PairList) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
func (p PairList) Less(i, j int) bool { return p[i].Value < p[j].Value }
func (ps *PeerSet) List() []Peer {
  // Return all peers, sorted in rank order

	p := make(PairList, len(ps.rankings))

	i := 0
	for _, peer := range ps.peers {
    if rank, ok := ps.rankings[peer.Id]; ok {
      p[i] = Pair{peer, rank}
      i++
    }
	}
	
	sort.Sort(p)

  var peerList []Peer = []Peer{}
  for _, item := range p {
    peerList = append(peerList, item.Key)
  }
  return peerList
}
func (ps *PeerSet) ListOthers() []Peer {
  var peerList []Peer
  for _, peer := range ps.List() {
    if peer != ps.Me {
      peerList = append(peerList, peer)
    }
  }
  return peerList
}
