# Deploy the OSC CCM
 
## Generate and apply the secret 
```
	cat ./deploy/secret_osc.yaml | \
	sed "s@AWS_ACCESS_KEY_ID@\"${AWS_ACCESS_KEY_ID}\"@g" 			  | \
	sed "s@AWS_SECRET_ACCESS_KEY@\"${AWS_SECRET_ACCESS_KEY}\"@g"    | \
	sed "s@AWS_DEFAULT_REGION@\"${AWS_DEFAULT_REGION}\"@g"		  | \
	sed "s@AWS_AVAILABILITY_ZONES@\"${AWS_AVAILABILITY_ZONES}\"@g"  | \
	sed "s@OSC_ACCOUNT_ID@\"${OSC_ACCOUNT_ID}\"@g" 				  | \
	sed "s@OSC_ACCOUNT_IAM@\"${OSC_ACCOUNT_IAM}\"@g"				  | \
	sed "s@OSC_USER_ID@\"${OSC_USER_ID}\"@g" 						  | \
	sed "s@OSC_ARN@\"${OSC_ARN}\"@g" > apply_secret.yaml
	
	cat apply_secret.yaml
	
	/usr/local/bin/kubectl delete -f apply_secret.yaml --namespace=kube-system
	/usr/local/bin/kubectl apply -f apply_secret.yaml --namespace=kube-system
```

## Deploy the CCM deamonset

```
	IMAGE_SECRET=registry-dockerconfigjson && \
	IMAGE_NAME=registry.kube-system:5001/osc/cloud-provider-osc && \
	IMAGE_TAG=v1 && \
	SECRET_NAME=osc-secret 
	helm del --purge k8s-osc-ccm --tls
	helm install --name k8s-osc-ccm \
		--set imagePullSecrets=$IMAGE_SECRET \
		--set oscSecretName=$SECRET_NAME \
		--set image.repository=$IMAGE_NAME \
		--set image.tag=$IMAGE_TAG \
		deploy/k8s-osc-ccm --tls
		
	kubectl get pods -o wide -A -n kube-system | grep osc-cloud-controller-manager

```