echo "restart oss-mysql container ..."
sudo docker restart oss-mysql
sleep 1s
echo "restart golang container ..."
sudo docker restart oss-go-docker
echo "finish"
