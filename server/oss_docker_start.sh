echo "start oss-mysql container ..."
cur_dir=$(pwd)"/data"
sudo docker run -itd --name oss-mysql -p 3310:3306 -v $cur_dir:/var/lib/mysql -e MYSQL_ROOT_PASSWORD=123456 oss-mysql
sleep 1s
echo "start golang container ..."
sudo docker run -p 10009:10008  --name oss-go-docker -v $(pwd)"/localfs_oss":/tmp/localfs_oss --link oss-mysql -e max_size=$1 riverpass
echo "finish"
