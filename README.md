# Minimal IPNI Publisher

This is a small stand-alone service.
It exposes a webserver interface, and is a libp2p bitswap peer. Requests to the web serve will cause the publisher to take the following steps:

1. A random 1kb of data will be created into a 'block' of data.
2. The block of data will be added to a local data store, and will be made available over bitswap.
3. The CID of the block will be advertised as available by this publisher to the IPNI network indexers
4. The CID of the block will be returned to the caller.

This provides a simple way to get a new, unique CID that is available over the IPNI network.


## License

[SPDX-License-Identifier: Apache-2.0 OR MIT](LICENSE.md)
