/// non concurent map based on https://github.com/suncat2000/hashmap
package util

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strconv"
)

const (
	growLoadFactor float32 = 0.75
)

// Key interface
type Key interface{}

// HashMaper interface
type HashMaper interface {
	Set(key Key, value interface{}) error
	Get(key Key) (value interface{}, err error)
	Count() int
}

// HashMap
type HashMap struct {
	hashFunc         func(blockSize int, key Key) (hashKey uint, bucketIdx uint) // hash func
	defaultBlockSize int                                                         // buckets block size
	buckets          []*Bucket                                                   // buckets for chains
	size             int                                                         // size of hash map
	halfSlice        bool                                                        // half slice used in buckets
}

// Bucket
type Bucket struct {
	hashKey uint
	key     Key
	value   interface{}
	next    *Bucket
}

// KeyValue
type KeyValue struct {
	key   Key
	value interface{}
}

// New HashMap.
func NewHashMap(blockSize int, fn ...func(blockSize int, key Key) (hashKey uint, bucketIdx uint)) HashMaper {
	//	log.Printf("New\n")
	hashMap := new(HashMap)
	hashMap.defaultBlockSize = blockSize
	hashMap.buckets = make([]*Bucket, hashMap.defaultBlockSize)
	hashMap.size = 0
	hashMap.halfSlice = true

	if len(fn) > 0 && fn[0] != nil && isFunc(fn[0]) {
		//fmt.Println(isFunc(fn[0]))
		hashMap.hashFunc = fn[0]
	} else {
		hashMap.hashFunc = hashFunc
	}

	return hashMap
}

// Get
func (self *HashMap) Get(key Key) (value interface{}, err error) {
	hashKey, bucketIdx := self.hashFunc(len(self.buckets), key)
	bucket := self.buckets[bucketIdx]
	for bucket != nil {
		if bucket.hashKey == hashKey && reflect.DeepEqual(key, bucket.key) {
			return bucket.value, nil
		}

		bucket = bucket.next
	}

	return nil, errors.New("Key not found!")
}

// Set
func (self *HashMap) Set(key Key, value interface{}) error {
	if self.loadFactor() >= growLoadFactor {
		//log.Printf("grow %d %d %d\n", self.loadFactor(), len(self.buckets), self.size)
		self.grow()
	}

	hashKey, bucketIdx := self.hashFunc(len(self.buckets), key)
	head := self.buckets[bucketIdx]
	self.buckets[bucketIdx] = &Bucket{hashKey, key, value, head}
	self.size++

	return nil
}

// Count
func (self *HashMap) Count() int {
	return self.size
}

// Function for calculate load factor
func (self *HashMap) loadFactor() float32 {
	return float32(self.size) / float32(len(self.buckets))
}

// Rehash buckets
func (self *HashMap) rehash(blockSize int) {
	//	log.Printf("rehashBuckets %d\n", len(buckets))
	buckets := make([]*Bucket, blockSize)
	for i, bucket := range self.buckets {
		for bucket != nil {
			bucketIdx := bucket.hashKey % uint(blockSize)
			head := buckets[bucketIdx]
			buckets[bucketIdx] = &Bucket{bucket.hashKey, bucket.key, bucket.value, head}
			bucket = bucket.next
		}
		self.buckets[i] = nil
	}
	self.buckets = buckets
}

// Grow
func (self *HashMap) grow() {
	//log.Printf("grow\n")
	blockSize := len(self.buckets) * 2
	if self.defaultBlockSize >= blockSize {
		blockSize = self.defaultBlockSize
	}
	self.rehash(blockSize)
}

func isFunc(v interface{}) bool {
	return reflect.TypeOf(v).Kind() == reflect.Func
}

// HASH FUNCTION

func writeValue(buf *bytes.Buffer, val reflect.Value) {
	switch val.Kind() {
	case reflect.String:
		buf.WriteByte('"')
		buf.WriteString(val.String())
		buf.WriteByte('"')
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		buf.WriteString(strconv.FormatInt(val.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		buf.WriteString(strconv.FormatUint(val.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		buf.WriteString(strconv.FormatFloat(val.Float(), 'E', -1, 64))
	case reflect.Bool:
		if val.Bool() {
			buf.WriteByte('t')
		} else {
			buf.WriteByte('f')
		}
	case reflect.Ptr:
		if !val.IsNil() || val.Type().Elem().Kind() == reflect.Struct {
			writeValue(buf, reflect.Indirect(val))
		} else {
			writeValue(buf, reflect.Zero(val.Type().Elem()))
		}
	case reflect.Array, reflect.Slice, reflect.Map, reflect.Struct, reflect.Interface:
		buf.WriteString(fmt.Sprintf("%#v", val))
	default:
		_, err := buf.WriteString(val.String())
		if err != nil {
			panic(fmt.Errorf("unsupported type %T", val))
		}
	}
}

// Hash function, return bucket index
func hashFunc(blockSize int, key Key) (hashKey uint, bucketIdx uint) {
	var buf bytes.Buffer
	writeValue(&buf, reflect.ValueOf(key))

	h := djb2Hash(&buf)
	//h := jenkinsHash(&buf)

	return h, (h % uint(blockSize))
}

func djb2Hash(buf *bytes.Buffer) uint {
	var h uint = 5381
	for _, r := range buf.Bytes() {
		h = (h << 5) + h + uint(r)
	}

	return h
}

func jenkinsHash(buf *bytes.Buffer) uint {
	var h uint
	for _, c := range buf.Bytes() {
		h += uint(c)
		h += (h << 10)
		h ^= (h >> 6)
	}
	h += (h << 3)
	h ^= (h >> 11)
	h += (h << 15)

	return h
}
