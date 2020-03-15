# ttlcache

The ttl cache is a cache with a key level time to live (ttl).

## Why ttlcache

As part of another, more advanced in memory caching mechanism (under development), an in memory backend cache, was required. No real candidates stood out, so the decision was made to implement a simple ttl based cache.

## Inner workings

The cache uses 2 maps to process the data, essentially doubling the write ttl, but reducing the read with about 15-20%. Since caches have their main benefits in read oriented loads, the suggested usage (and assumption) is that the cache is used in read oriented situations, thus allowing us to ignore the write load (See alternative cache package in the `alternative` directory for the implementation with a single path).

The cache supports multiple masterkeys with their own configuration and callback functions. All the required memory is initialized on demand, costing a few extra ns per read, but saving on memory in low usage scenarios

### Data overflow

The cache is not protected against overflow of data: It does not tell you if requests to write to the cache end in nothing! It also does not stop working for values which fit in the cache. For cache sizing, a statistics function is supplied and will dump information in the log at every ttl expire cleanup.

## Usage

### To be implemented interfaces

Internally the cache works with []byte. Since the key type is unknown, and the golang to byte code is rather elaborate, a specific key to byte implementation should be faster. Implement the keyFunctions interface KeyToByte function to handle this.

For example for an uint32:

```golang
type f struct{}

// KeyToByte - Parses specific key as provided by the code to []byte
func (*f) KeyToByte(key interface{}) []byte {
    var bs := make([]byte, 4)
    binary.LittleEndian.PutUint32(bs, key.(uint32))
    return bs
}
```

By using a uint32 from the start for the key (in this scenario), the key conversion is optimized in just a few lines. By thinking of what key type to use, this function can be kept extremely fast, which is relevant for overall performance.

### Store data in the cache

Use the `Write` function:

```golang
Write(key, value, time.Duration, masterKey)
```

### Read data from the cache

Call the `Read`:

```golang
Read(key,masterkey)
```

## Benchmarks & lies

Benchmark numbers from macbookpro 2019 (1.4GHz quad-core 8th-gen Intel Core i5 processor, 8GB).

### Reads only

Only reads have been benchmarked. A read/write benchmark would be due to the writes and way locking works with go maps, just add the write time to the read time.

The initial partitioning (on masterkey) slows down reads. The partitioning is introduced to reduce lock contention, but does not work in the alternative cache package due to the way the single map has to lock, and the lack of a place to embed a lock in a struct.
The read performance is further dependent on the entries in each map backing the cache. When the data distribution is known to be rather even (Every partition will contain about the same data), an up to x256 times smaller number of entries can be used to initialize (256=max int in 1 byte). This test was done assuming a very asymptotic distribution with entries at 100k (static value). This seems to set the limitation. A test with optimized values for keys and entries, leads to a higer read performance:

| entries | keys | ns/read
|---|---|---
| 1000 | 100k | 92

As usual: Compare this with your favourite caching library/object database/etc to find that that is faster/slower.

## Is C faster

Yes. The memory access and prevention of garbage collection, will make for a faster C implementation of the same code. Time can also be saved on key conversion (just grep the byte from memory).