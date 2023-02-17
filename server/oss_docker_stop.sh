echo "stop oss-mysql container ..."
sudo docker stop oss-mysql
sleep 1s
echo "stop golang container ..."
sudo docker stop oss-go-docker
echo "finish"