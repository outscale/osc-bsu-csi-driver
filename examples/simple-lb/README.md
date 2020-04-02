# Simple lb creation
 
This example creates a deployment named echoheaders on your cluster, which will run a single replica 
of the echoserver container, listening on port 8080.
Then create a Service that exposes our new application to the Internet over an Outscale Load Balancer unit (LBU).

- Create ns

```
$ /usr/local/bin/kubectl create namespace simple-lb
namespace/simple-lb created
```

- Create bucket for logs 
```
$ aws s3 mb s3://ccm-examples  --endpoint https://osu.eu-west-2.outscale.com
make_bucket: ccm-examples
```

- Deploy the application ,which a simple server that responds with the http headers it received, along with the Loadbalancer

```
$ /usr/local/bin/kubectl apply  -f examples/simple-lb/specs/
	deployment.apps/echoheaders created
	service/echoheaders-lb-public created
	
$kubectl get all -n simple-lb
NAME                               READY   STATUS    RESTARTS   AGE
pod/echoheaders-5465f4df9d-wxht2   1/1     Running   0          5m20s

NAME                            TYPE           CLUSTER-IP     EXTERNAL-IP                                                             PORT(S)        AGE
service/echoheaders-lb-public   LoadBalancer   10.32.187.30   a4fd6f97708b94d288e9986f98df61da-322867284.eu-west-2.lbu.outscale.com   80:32363/TCP   5m20s

NAME                          READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/echoheaders   1/1     1            1           5m21s

NAME                                     DESIRED   CURRENT   READY   AGE
replicaset.apps/echoheaders-5465f4df9d   1         1         1       5m21s
```

- Validate the LB was created and the endpoint is available

```	
$ kubectl get service  -n simple-lb echoheaders-lb-public
NAME                    TYPE           CLUSTER-IP     EXTERNAL-IP                                                             PORT(S)        AGE
echoheaders-lb-public   LoadBalancer   10.32.187.30   a4fd6f97708b94d288e9986f98df61da-322867284.eu-west-2.lbu.outscale.com   80:32363/TCP   33s
```
- Check logs under  buckets created  and its content
```
aws s3 ls --recursive s3://ccm-examples/ --endpoint https://osu.eu-west-2.outscale.com

```
- Wait for the lb to be ready  and then check it is running and forwarding traffic

```		
$ curl http://a4fd6f97708b94d288e9986f98df61da-322867284.eu-west-2.lbu.outscale.com

Hostname: echoheaders-5465f4df9d-wxht2

Pod Information:
	-no pod information available-

Server values:
	server_version=nginx: 1.13.3 - lua: 10008

Request Information:
	client_address=172.19.91.102
	method=GET
	real path=/
	query=
	request_version=1.1
	request_scheme=http
	request_uri=http://a4fd6f97708b94d288e9986f98df61da-322867284.eu-west-2.lbu.outscale.com:8080/

Request Headers:
	accept=*/*
	host=a4fd6f97708b94d288e9986f98df61da-322867284.eu-west-2.lbu.outscale.com
	user-agent=curl/7.29.0

Request Body:
	-no body in request-
```

- Cleanup resources:

```
/usr/local/bin/kubectl delete  -f examples/simple-lb/specs/
```



