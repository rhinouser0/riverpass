# Rhino Cache: Local Disk Based Object Storage Cache

This project introduced a handy local cache to store hot content at local storage. 

We provided docker image which comes with the binary and required MySQL instance. After instance starts, the cache usage is as simple as wget http://localhost:10009/getFile?url=https://deploee.oss-cn-shanghai.aliyuncs.com/resnet18.tar, in which, the cache is deployed at local host, and the url(https://deploee.oss-cn-shanghai.aliyuncs.com/resnet18.tar) is a reference to a model store at Aliyun OSS with open-read access.

The persistence module borrowed some thoughts from Haystack, the well known BLoB storage design by Facebook; The cache algorithm currently is a naive LRU on blocks of aggregated BLoBs. 

We use MySQL and the Haystack like design to make sure the program is resumable.

We hope to continuous improve our caching algorithm, thread model, etc. Also welcome to contribute. 
