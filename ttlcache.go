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
	sync.RWMutex
	dataSets       map[interface{}]interface{}
	dataManagement map[interface{}]*data
	keys           int
}

type data struct {
	setTime time.Time
	ttl     time.Duration
}

type keySet struct {
	m  *ttlManagement
	k3 interface{}
}

// mainData struct setup makes it possible to read the base (masterKey) only once, reducing the read time with a few ns/read
type mainData struct {
	functions ttlFunctions
	// 256 memory partitions (1 byte)
	data [256]*ttlManagement
}

var (
	ttlMem         = make(map[string]*mainData) // Interface as a key might not be static: If a pointer is passed in, no-one will ever have the same pointer again.
	masterSize     = make(map[string]int)
	errKeyNotFound = errors.New("Key not found")
	mutex          = &sync.RWMutex{}
)

func init() {
	go expire()
}

// InitCache - Stores config value entries for later use
// InitCache has to be called for all used masterkeys at the start of the program since the rest of the program has no lock protection on the supposedly initialized slices
func InitCache(entries int, masterKey string, k ttlFunctions) {
	mutex.Lock()
	masterSize[masterKey] = entries
	m := &mainData{}
	ttlMem[masterKey] = m
	md := m.data
	for i := 0; i <= 255; i++ {
		md[i] = &ttlManagement{}
	}
	m.data = md
	m.functions = k
	mutex.Unlock()
}

// Stats - Internal statistics for performance analysis
func Stats() {
	for k, v := range ttlMem {
		log.Printf("Master key: %s, partitions %d", k, len(v.data))
		for i, j := range v.data {
			log.Printf("Key: %s, partition %d, size %d, registered keys %d", k, i, len(j.dataSets), j.keys)
		}
	}
}

// Read - read a key from the cache, exact key expiration
// With specific locking on the pointer, and with the array of pointers being static (read only after init), this code can be used for parallel reads with minimum blocking
func Read(key interface{}, masterKey string) (interface{}, error) {
	// To skip locking here requires essentially all cache masterkeys to be initialized (design trade off)
	z := ttlMem[masterKey]
	k := z.functions.KeyToByte(key)
	if len(k) == 0 {
		return nil, errKeyNotFound
	}
	// With the lock at struct level, we lock only one pointer for the read operation, so no mutex required here: Gets the read time down with about 2-4ns/read
	// Again, all slices need to be initialized to be allowed to lock this late
	q := z.data[k[0]]
	q.RLock()
	// while defer q.RUnlock() is go idiomatic and correct, it is slow: Timing of code using specific unlock at the independent locations improved 15ns per read
	// We need a copy value of the data so that we can unlock the struct (so some overhead in memory management)
	v := q.dataSets[key]
	if v != nil {
		// Exact expiration adds about 22ns per read, so not used here (slight reduction off functionality vs arbitrary caching duration)
		// if time.Since(v.setTime) > v.ttl {
		// 	return nil, errKeyNotFound
		// }
		q.RUnlock()
		return v, nil
	}
	q.RUnlock()
	return nil, errKeyNotFound
}

// Write - Write data to the cache
func Write(key interface{}, value interface{}, ttl time.Duration, masterKey string) {
	// Requirement: All slices are initialized: No locking required
	z := ttlMem[masterKey]
	n := z.data[z.functions.KeyToByte(key)[0]] // The given subindex (used to reduce lock contention on write)
	// By using n.keys instead of len(n.dataSets), a faster accesspath to statistics is used (impact not tested)
	// With the lock at struct level, we lock only one pointer for the slow operation
	n.Lock()
	if n.keys < masterSize[masterKey] {
		if n.dataSets == nil {
			n.dataSets = make(map[interface{}]interface{})
			n.dataManagement = make(map[interface{}]*data)
		}
		n.dataSets[key] = value
		n.dataManagement[key] = &data{setTime: time.Now(), ttl: ttl}
		n.keys = n.keys + 1
	}
	n.Unlock()
}

// expire - Manages the expiration of data in the cache
// expire is a go routine which once per time interval checks the state of the cache
func expire() {
	for {
		time.Sleep(10 * time.Second)
		var expiredData []*keySet
		// Iterate over all cached sets using the TTL. Delete all expired records
		for _, v := range ttlMem {
			// Iterate over sub sets
			for _, m := range v.data {
				m.RLock()
				// Iterate over stored record time
				for q, t := range m.dataManagement {
					// use time.Since since every ttl and setTime can be different
					if time.Since(t.setTime) > t.ttl {
						// Map has last been
						expiredData = append(expiredData, &keySet{m, q})
					}
				}
				m.RUnlock()
			}
		}
		// Use the collected data in the expiredData array to delete all data from the ttlMem set which is expired
		if len(expiredData) > 0 {
			for _, v := range expiredData {
				v.m.Lock()
				delete(v.m.dataSets, v.k3)
				delete(v.m.dataManagement, v.k3)
				v.m.keys--
				v.m.Unlock()
			}
		}
	}
}
