/*
TTL cache:
- Objects live max the indicated TTL in the cache in a limited size map
- Cache is implemented with go maps (Garbage collection is ignored for the moment)
- To reduce locking, cache is split in 256 segments (key[0] is the overarching index)
- Data in cache is []byte
- Expire is done only once per time interval. No TTL is checked on read, so TTL is an approximate value
*/
package ttlcache

import (
	"errors"
	"log"
	"sync"
	"time"
)

type ttlFunctions interface {
	// KeyToByte - Instead of using dynamic keys, a key to []byte function is to be implemented by the user of this package
	// The function is a more direct (potentially faster) way to convert the key to a []byte
	// Its implementation also makes clear that passing a pointer as a key might be a bad idea (while storing the pointer will work, retrieval will never work...)
	KeyToByte(key interface{}) []byte
}

type ttlManagement struct {
	dataSets map[interface{}]*data
	keys     int
}

type data struct {
	setTime time.Time
	ttl     time.Duration
}

type keySet struct {
	k1 string
	k2 byte
	k3 interface{}
}

var (
	mem            = make(map[string]map[byte]map[interface{}]interface{}) // Interface as a key might not be static: If a pointer is passed in, no-one will ever have the same pointer again.
	ttlMem         = make(map[string]map[byte]*ttlManagement)
	masterSize     = make(map[string]int)
	keyFunctions   = make(map[string]ttlFunctions)
	errKeyNotFound = errors.New("Key not found")
	mutex          = &sync.RWMutex{}
)

func init() {
	go expire()
}

// InitCache - Stores config value entries for later use
func InitCache(entries int, masterKey string, k ttlFunctions) {
	masterSize[masterKey] = entries
	keyFunctions[masterKey] = k
}

// Stats - Internal statistics for performance analysis
func Stats() {
	for k, v := range ttlMem {
		log.Printf("Key: %s, partitions %d", k, len(v))
		for i, j := range v {
			log.Printf("Key: %s, partition %d, size %d, registered keys %d", k, i, len(j.dataSets), j.keys)
		}
	}
}

// Read - read a key from the cache, eventual consistency, flexible TTL
func Read(key interface{}, masterKey string) (interface{}, error) {
	mutex.RLock()
	k := keyFunctions[masterKey].KeyToByte(key)
	val := mem[masterKey][k[0]][key]
	mutex.RUnlock()
	if val == nil {
		return nil, errKeyNotFound
	}
	return val, nil
}

// Write - Write data to the cache
func Write(key interface{}, value interface{}, ttl time.Duration, masterKey string) error {
	// Write data to map:
	mutex.Lock()
	k := write(key, value, masterKey)
	writeTTL(key, ttl, masterKey, k)
	mutex.Unlock()
	return nil
}

// write - Write the data to the map
func write(key interface{}, value interface{}, masterKey string) byte {
	m := mem[masterKey]
	if m == nil {
		// Order of assignment: By assigning value to (re-used) temp value, and then to mem[masterKey] location, a map lookup is saved
		// Also applied in all other assignments
		m = make(map[byte]map[interface{}]interface{})
		mem[masterKey] = m
	}
	k := keyFunctions[masterKey].KeyToByte(key)
	q := k[0] // Shaves 1s/10mln operations
	n := m[q] // The given subindex (used to reduce lock contention on write)
	if n == nil {
		n = make(map[interface{}]interface{}, masterSize[masterKey])
		mem[masterKey][q] = n
	}

	n[key] = value
	return q
}

// writeTTL - Updates the TTL data
func writeTTL(key interface{}, ttl time.Duration, masterKey string, k byte) {
	m := ttlMem[masterKey]
	if m == nil {
		m = make(map[byte]*ttlManagement)
		ttlMem[masterKey] = m
	}
	n := m[k] // The given subindex (used to reduce lock contention)
	if n == nil {
		n = &ttlManagement{
			dataSets: make(map[interface{}]*data, masterSize[masterKey]),
			keys:     0,
		}
		ttlMem[masterKey][k] = n
	}
	// By using n.keys instead of len(n.dataSets), a faster accesspath to statistics is used (impact not tested)
	if n.keys < masterSize[masterKey] {
		n.dataSets[key] = &data{setTime: time.Now(), ttl: ttl}
		n.keys = n.keys + 1
	}
}

// expire - Manages the expiration of data in the cache
// expire is a go routine which once per time interval checks the state of the cache
func expire() {
	for {
		time.Sleep(10 * time.Second)
		var expiredData []*keySet
		// Iterate over all cached sets using the TTL. Delete all expired records
		for r, v := range ttlMem {
			// Iterate over sub sets
			for s, m := range v {
				// Iterate over stored record time
				for q, t := range m.dataSets {
					if time.Since(t.setTime) > t.ttl {
						// Map has last been
						mutex.Lock()
						data := mem[r][s]
						expiredData = append(expiredData, &keySet{k1: r, k2: s, k3: q})
						delete(data, q)
						m.keys--
						mutex.Unlock()
					}
				}
			}
		}
		// Use the collected data in the expiredData array to delete all data from the ttlMem set which is expired
		if len(expiredData) > 0 {
			mutex.Lock()
			for _, v := range expiredData {
				k := ttlMem[v.k1][v.k2]
				delete(k.dataSets, v.k3)
			}
			mutex.Unlock()
		}
		log.Println("Expiring data")
		Stats()
	}
}
