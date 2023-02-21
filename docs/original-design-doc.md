# Riverpass Cache Design
## Persistence 
1. We use a set of files together the persist data on HDD or SSDs. These files are grouped by every 2, and each of these 2 are called a "triplet". The name "triplet" comes from the original 3-file design with another manifest file, which is removed in current master version. Files in a triplet orchestrate together to perform system crash resumable BLoB data persistence. It has some similarity of "Haystack", the Facebook blob storage base born in last decade.
2. Within a triplet, index file and the blob content files are logically binding to 1 same set of blobs.
3. In folder `/tmp/localfs_oss/`, file with prefix "idx_h" are index files, which contains the index list of blobs in the real blob content files. In the future, we may open multiple index header file belong to 1 index file to improve the write&read bandwidth.
4. In folder `/tmp/localfs_oss/`, file with prefix "blobs_" are blob files, which contains the contents of real blobs, in one by one manner.
5. We shall admit that the "triplet" design has some degree of over design due to it's originally designed for a way larger closed-sourced file system. We grafted its single host persistence store module and open-source it for now to support other urgent project. 

## Cache Read flow
When user request a file read, the data doesn't exist, a background thread/go-routine shall trigger a OSS read, and write to the file handler and eventually into physical blob handler.
When user request a file read and it exists, the file cache holder shall return object. It shall fetch data by calling FileReader, fetching from cached triplet file. Certain cache score/weight shall be re-caculated internally.

## Metadata
1. Main table is oss_files table. All files appear in the cache only consist of 1 blob (we will improve the download part to chop files ~1GB into blobs so it's easy for downloading from OSS). Doesn't need to modify any schema of files or blobs table, directly use blob id records to link to the blob object at triplet, on disk.
2. Use fid field to store the file name, since file name in cache is unique, and fid is indexed.
3. Store the most important information at file_meta field: blob id, token that consist of triplet id+blob id and the size (store at RangeCode.End). All other fields can be left blank, however, we shall minimize the line of code change so sometimes we still fill in fields because old code does.
4. Store the owner at mysql files table, add an DB secondary index on owner field. This is for cache eviction metadata GC.

## Cache Item Structure
1. The blob links to objects written into triplets, similar to how do we manage data in physical blob handler. But to be noted here, we shall use linked list to manage triplet object in memory rather than arrays to implement the cache. It is highly recommended to modify the code in original physical blob holder, so that we can reuse existing system as much as possible. Actually linked list style triplets doesn't hurt performance much in normal Rhino FS.
2. We categorize object into large object and normal object two types. Large object doesn't need aggregation and they themselves are elements of LRU cache. Normal object need aggregation to achieve better disk performance.
3. For taking writes, we enqueue new object at head for those newly queried but cache missed data, regardless normal object list or large object list. Note the normal object list always uses only the first triplet to take writes, to reduce the write fanout.
4. For taking read: 
    - For large object list, we move last read or enqueued elements to head, so that when certain criteria meets, we remove tail element.
    - For normal object list, we keep a read frequency score at each triplet object, based upon this score we sort the triplet list: the triplet having most frequent access (or "higher scored") are stored at second position in list. 
5. Whenever we need cache eviction (object(s) at tail), we first uses triplet id to query files at files table, use the triplet id as "owner". Then purge all records in files DB, then remove the tail.