# Structure of Blob Management
Index file, manifest file and the blob content files are logically binding to 1 same set of blobs.
File with prefix "idx_h" are index files, which contains the index list of blobs in the real blob content files. In the future, we may open multiple index header file belong to 1 index file to improve the write&read bandwidth.
File with prefix "blobs_" are blob files, which contains the contents of real blobs, in one by one manner.
File with prefix "mf_" are manifest files, similar to write ahead log files, it contains blob actions related to certain blob content or idx_h file. 

mf_ may still grow even idx file has closed.
After idx file is closed, it may reopen and be groomed due to compaction.
mf_ will never close unless user deleted everything in this blob content file, or this blob content file is migrated and destroyed.
