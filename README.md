# go-ttlcache

go-ttlcache is a cache with a key level time to live (ttl).

## Why go-ttlcache

As part of another, more advanced in memory caching mechanism (under development), an in memory backend cache, was required. No real candidates stood out, so the decision was made to implement a simple ttl based cache.

## Inner workings

The cache uses a map to process the data. The initial map is for a high level key (aka masterKey), the sceond level in the data is an array with 256 entries for data distribution. The 3rd level is a datanode with registered to it the required key functions, size of the current data set, and the data itself.

The cache supports multiple masterkeys with their own configuration and callback functions. All the required memory is initialized on demand, creating a stable data access time.

### Data overflow

The cache is not protected against overflow of data: It does not tell you if write requests to the cache end in nothing! For cache sizing, a statistics function is supplied and will dump information in the log at every ttl expire cleanup.

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

### Performance analysis

The initial partitioning (on masterkey) slows down reads. Partitioning is introduced to reduce lock contention.
The read performance is further dependent on the entries in each map backing the cache. When the data distribution is known to be rather even (Every partition will contain about the same data), an up to x256 times smaller number of entries can be used to initialize (256=max int in 1 byte, which is the designed data partitioning). 
This test was done assuming a very even distribution with of entries with 100k entries total (static value).
Every change the test was ran and optimizations were made, leading in the current version to:

| max entries | keys | ns/read
|---|---|---
| 1000 | 100k | 108

As usual: Compare this with your favourite caching library/object database/etc to find that that is faster/slower.

## Is C faster

Yes. The memory access and prevention of garbage collection, will make for a faster C implementation of the same code. Time can also be saved on key conversion (just grep the byte from memory).
