# k8s安装

在每台服务器中安装k8s,使用containerd作为容器运行时
在每个master节点(role=master)上安装k8s control plane
在每个node节点(role=worker)上安装k8s node
需要考虑到节点上既有role=master也有role=worker的情况

k8s的二进制文件已经下载至/ar/tar/kubernetes-server-linux-amd64.tar.gz目录下
在前序步骤中,这个文件会被拷贝至目标节点的/tmp/ar/tar/kubernetes-server-linux-amd64.tar.gz下
CA证书已生成并存放于/ar-data/pki/目录下,在前序步骤中,这些证书已被拷贝至目标节点的/tmp/ar/pki/目录下

在etcd安装完成后,执行以下步骤:
1. 使用cfssl结合/ar-data/pki/ca-config.json和/ar-data/pki/ca-csr.json和已有ca证书生成apiserver,scheduler,controller-manager证书和私钥,存放于/ar-data/pki/下,其中apiserver的证书中需要包含lvs-care的虚拟IP地址(10.103.97.12:6443)
2. 使用cfssl结合/ar-data/pki/ca-config.json和/ar-data/pki/ca-csr.json和已有ca证书生成kubelet证书和私钥,存放于/ar-data/pki/下
3. 使用cfssl结合/ar-data/pki/ca-config.json和/ar-data/pki/ca-csr.json和已有ca证书生成kube-proxy证书和私钥,存放于/ar-data/pki/下
4. 使用cfssl结合/ar-data/pki/ca-config.json和/ar-data/pki/ca-csr.json和已有ca证书生成admin证书和私钥,存放于/ar-data/pki/下
5. 使用cfssl结合/ar-data/pki/ca-config.json和/ar-data/pki/ca-csr.json和已有ca证书生成front-proxy-client证书和私钥,存放于/ar-data/pki/下
6. 将生成的证书和私钥拷贝至目标节点的/tmp/ar/pki/目录下
7. 在master节点上安装k8s control plane,master上连接etcd节点
8. 在node节点上安装k8s node

任务要求:
1. 在/home/ubuntu/ar/pipelines/containerd-k8s-1.35.0-amd64-v2/steps/base/ar/scripts目录下写入generate-certs.sh脚本,用于生成证书
2. 在/home/ubuntu/ar/pipelines/containerd-k8s-1.35.0-amd64-v2/steps/base/ar/scripts目录下写入install-master.sh脚本,用于在master节点上安装k8s control plane
3. 在/home/ubuntu/ar/pipelines/containerd-k8s-1.35.0-amd64-v2/steps/base/ar/scripts目录下写入install-node.sh脚本,用于在node节点上安装k8s node
4. node.json中labels需要写入k8s的node资源label中
