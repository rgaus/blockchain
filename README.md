# My Toy Blockchain

I've been curious about how blockchain-based systems like ethereum work. So, I built my own little
toy version to better understand some of the semantics of those systems.

## Getting Started
To build the demo locally:
```bash
$ go build .
```

### Creating Nodes

To give this demo a shot, first, a new "node" process has to be created. It is initialized without
knowledge of any other nodes, so it initializes a brand new blockchain from scratch, creating a
random set and number of transactions and then forming a genesis block with them. Note that all
nodes are also miners - there is one type of node to keep things simple.

Note that in the below, PORT=4000 defines the port that the node will listen on, and the value
passed to `--address` defines the publicly facing address the node can be reached at. For example,
if this node was behind a reverse proxy of some sort, these values may need to be different.
```
$ PORT=4000 ./blockchain node --address http://localhost:4000
No valid peers found.
Mine Status: 0
Running on :4000
Mine Status: 1000
Mine Status: 2000
Mine Status: 3000
(... lines removed for brevity ...)
Mine Status: 30000
Mine Status: 31000
Mine Status: 32000
Created genesis block: 00003f9c6bac6056c682ed0e9fa11c568f2011c689423675702c75788609a3e2
```

Next, I'll start another node, giving it context on the first node so it can form a peer-to-peer
network:
```bash
$ PORT=4001 ./blockchain node --address http://localhost:4001 --peers http://127.0.0.1:4000
Running on :4001
Setting up peerset...
New peer 60f76e2c-4666-4a96-be28-2976302c01de (address http://localhost:4000) found!
Checking to make sure all peers are healthy...
Checking to make sure all peers are healthy...done
Number of healthy peers: 2
Trying to aquire more peers to get to 10
2025/04/11 11:00:48 "GET http://localhost:4001/v1/me HTTP/1.1" from [::1]:57639 - 200 80B in
83.916µs
Number of peers: 2
Peerset configured, 2 valid peer(s) found
Begin syncing chain from peer 60f76e2c-4666-4a96-be28-2976302c01de
Got chain data from peer 60f76e2c-4666-4a96-be28-2976302c01de
Fetching data from 1 appendage(s)...
Added head block 00003f9c6bac6056c682ed0e9fa11c568f2011c689423675702c75788609a3e2 in appendage
Fetched 1 block(s) in appendage
2025/04/11 11:00:50 "GET http://localhost:4001/v1/me HTTP/1.1" from [::1]:57639 - 200 80B in
64.708µs
2025/04/11 11:00:50 "GET http://localhost:4001/v1/peers HTTP/1.1" from [::1]:57639 - 200 172B in
60.375µs
2025/04/11 11:00:50 "GET http://localhost:4001/v1/me HTTP/1.1" from [::1]:57639 - 200 80B in
17.084µs
2025/04/11 11:00:50 "GET http://localhost:4001/v1/me HTTP/1.1" from [::1]:57639 - 200 80B in
12.167µs
Checking to make sure all peers are healthy...
Checking to make sure all peers are healthy...done
Number of healthy peers: 2
Trying to aquire more peers to get to 10
```

Create as many nodes as you'd like! As long as a new node is given a list of peers via `--peers`, it
will join the network and grow its list of healthy peers up to a maximum of 10. As nodes cycle on
and offline, each node will keep its peers list up to date to only contain healthy nodes.

### Submitting transactions
To submit a transaction, generating a private key is required. Note that the private key contains
information to derive the public key so for this demo they aren't stored separately.

```bash
$ ./blockchain generate --filename keyone.pem
```

Now, a `keyone.pem` file should be in your current directory, containing an armored RSA private key.

Now, the main event: to submit a transaction, run the below, pointing it to a node's address:
```bash
$ ./blockchain submit --address http://localhost:4000 --data 'hello world' --key keyone.pem
```

After running this, one of the nodes will begin processing the transaction and will craft a new
block containing it:
```bash
# From the logs of one of the above node processes!
Mined new block: &000049bec25a20cc265b018075d1f73fe3800b91c3af5c9d7c536f4fad28fedc 
```

Feel free to dig around in the REST api that the node process exposes to understand the state of the
system:
```
$ curl http://localhost:4000/v1/chain
$ curl http://localhost:4000/v1/block/<block hash>
$ # etc
```
