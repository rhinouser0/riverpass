<small> [简体中文](README_zh.md) | English </small>

## Ryno | [Documentation](docs/)
[![GitHub license](https://img.shields.io/badge/license-apache--2--Clause-brightgreen.svg)](./LICENSE)

---

A handy local disk based cache for hot content from remote storage. 

* Extremely simple command start and stop the cache.
* No heavy configuration steps with the remote cloud storages.
* Cache item persistence ability: previous items will be reloaded after server restart.

## Design
[Detailed design document](docs/original-design-doc.md)

## HowTo
* How to use
  * Enter `server` folder, run `./oss_docker_start.sh 100`, '100' means cache size 100MB. Cache data default flushes to /tmp/localfs_oss/ folder.
  * Use `wget <url>` command, replacing host path by localhost and cache port. eg.: `wget https://deploee.oss-cn-shanghai.aliyuncs.com/resnet18.tar`
  * Run `./oss_docker_stop.sh` to stop the cache. Data will be left on disk.
  * Run `./oss_docker_restart.sh` to restart the cache, data and their metadata will be loaded.
* How to build
  * Enter `server/holder` folder, run `./oss_start.sh` to build the go program and start server for debug.
* [How to contribute](docs/how-to-contribute.zh.md)

## Dependency
* MySQL 8.0
* Aliyun OSS SDK

## Coming Soon
- CI and test coverage
- Stale metadata GC in DB
- OSS download optimization
- Object service from other cloud provider
- Cache eviction algorithm improvement

## Contact Us
  * Issue: https://github.com/rhinouser0/ryno/issues
  * Email: rhino_fs@163.com

## License
- [Apache 2.0](LICENSE)