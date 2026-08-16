#!/bin/bash
docker build -t vertex-pet-service:latest -f Dockerfile .
docker save vertex-pet-service:latest > vertex-pet-service.tar
sshpass -p '***REMOVED-SEE-VT-112***' scp -o StrictHostKeyChecking=no vertex-pet-service.tar ppluemthpp@192.168.1.82:~/vertex-pet-service.tar
sshpass -p '***REMOVED-SEE-VT-112***' ssh -o StrictHostKeyChecking=no ppluemthpp@192.168.1.82 "echo '***REMOVED-SEE-VT-112***' | sudo -S k3s ctr images import ~/vertex-pet-service.tar -n k8s.io && echo '***REMOVED-SEE-VT-112***' | sudo -S k3s ctr -n k8s.io images tag docker.io/library/vertex-pet-service:latest 192.168.1.82:32000/vertex-pet-service:latest && echo '***REMOVED-SEE-VT-112***' | sudo -S k3s kubectl rollout restart deployment pet-service -n vertex"
