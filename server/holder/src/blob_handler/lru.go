package blob_handler

import (
	. "github.com/common/zaplog"
	_ "github.com/common/zaplog"
	"go.uber.org/zap"
	db_ops "holder/src/db_ops"
	"sync"
)

type Node struct {
	key   string
	value *Triplet
	prev  *Node
	next  *Node
}

type LruCache struct {
	dbOpsFile db_ops.DBOpsFile
	dict      sync.Map
	head      *Node
	tail      *Node
	size      int
	listLock  sync.Mutex
	rwLock    sync.RWMutex
}

func (c *LruCache) New() {
	c.size = 0
	c.head = new(Node)
	c.tail = new(Node)
	c.head.next = c.tail
	c.head.prev = c.tail
	c.tail.next = c.head
	c.tail.prev = c.head
}

func (c *LruCache) Get(key string) *Triplet {
	c.rwLock.RLock()
	defer c.rwLock.RUnlock()
	if v, ok := c.dict.Load(key); ok {
		c.moveToHead(v.(*Node))
		return v.(*Node).value
	}
	return nil
}

func (c *LruCache) Put(key string, value *Triplet) {
	c.rwLock.Lock()
	defer c.rwLock.Unlock()
	ZapLogger.Info("[LRU PUT] ", zap.Any("key", key))
	if v, ok := c.dict.Load(key); ok {
		v.(*Node).value = value
		c.moveToHead(v.(*Node))
	} else {
		c.size++
		node := new(Node)
		node.key = key
		node.value = value
		c.addToHead(node)
		c.dict.Store(key, node)
	}
}

func (c *LruCache) Evict() int64 {
	c.rwLock.Lock()
	defer c.rwLock.Unlock()
	ZapLogger.Info("[LRU PUT]  start evict")
	node := c.removeTail()
	//delete db item
	err := c.dbOpsFile.DeleteFileWithTripleIdInDB(node.key)
	if err != nil {
		ZapLogger.Error("DELETE FILE IN DB ERROR", zap.Any("error", err))
	}
	c.dict.Delete(node.key)
	c.size--
	deleteSize := DeleteTripletFilesOnDisk(node.key)
	ZapLogger.Info("[LRU PUT]  evict end", zap.Any("key", node.key))
	return deleteSize
}

func (c *LruCache) addToHead(node *Node) {
	c.listLock.Lock()
	defer c.listLock.Unlock()
	node.next = c.head.next
	c.head.next.prev = node
	c.head.next = node
	node.prev = c.head
}

func (c *LruCache) deleteNode(node *Node) {
	c.listLock.Lock()
	defer c.listLock.Unlock()
	node.prev.next = node.next
	node.next.prev = node.prev
}

func (c *LruCache) moveToHead(node *Node) {
	c.deleteNode(node)
	c.addToHead(node)
}

func (c *LruCache) removeTail() *Node {
	c.listLock.Lock()
	defer c.listLock.Unlock()
	node := c.tail.prev
	node.prev.next = node.next
	node.next.prev = node.prev
	return node
}

func (c *LruCache) GetSize() int {
	return c.size
}

func (c *LruCache) DeleteFromCache(key string) {
	c.rwLock.Lock()
	defer c.rwLock.Unlock()
	node, ok := c.dict.Load(key)
	if ok {
		c.deleteNode(node.(*Node))
		c.dict.Delete(node.(*Node).key)
	}
}
