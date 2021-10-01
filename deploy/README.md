# Deploy the OSC CCM
##
- The ccm needs AK/SK to interact with Outscale lbu and fcu API, so you can create an AK/SK using an eim user, for example, with a proper permission by attaching [a policy like](./example-eim-policy.json) 

## Generate and apply the osc-secret 
```
	OSC_ACCOUNT_ID=XXXXX		: the osc user id
	OSC_ACCOUNT_IAM=xxxx		: the eim user name  
	OSC_USER_ID=XXXX			: the eim user id
	OSC_ARN="XXXXX"				: the eim user orn
	AWS_ACCESS_KEY_ID=XXXX 		: the AK
	AWS_SECRET_ACCESS_KEY=XXXX 	: the SK
	AWS_DEFAULT_REGION=XXX		: the Region to be used
	
	cat ./deploy/secret_osc.yaml | \
	sed "s@AWS_ACCESS_KEY_ID@\"${AWS_ACCESS_KEY_ID}\"@g" | \
	sed "s@AWS_SECRET_ACCESS_KEY@\"${AWS_SECRET_ACCESS_KEY}\"@g" | \
	sed "s@AWS_DEFAULT_REGION@\"${AWS_DEFAULT_REGION}\"@g" | \
	sed "s@AWS_AVAILABILITY_ZONES@\"${AWS_AVAILABILITY_ZONES}\"@g" | \
	sed "s@OSC_ACCOUNT_ID@\"${OSC_ACCOUNT_ID}\"@g" | \
	sed "s@OSC_ACCOUNT_IAM@\"${OSC_ACCOUNT_IAM}\"@g" | \
	sed "s@OSC_USER_ID@\"${OSC_USER_ID}\"@g" | \
	sed "s@OSC_ARN@\"${OSC_ARN}\"@g" > apply_secret.yaml
	
	cat apply_secret.yaml
	
	/usr/local/bin/kubectl delete -f apply_secret.yaml --namespace=kube-system
	/usr/local/bin/kubectl apply -f apply_secret.yaml --namespace=kube-system
```

## Deploy the CCM deamonset


```
	#Using the manifest
	kubectl delete -f deploy/osc-ccm-manifest.yml
	kubectl apply -f deploy/osc-ccm-manifest.yml
	kubectl get pods -o wide -A -n kube-system | grep osc-cloud-controller-manager

	#Using helm3
	cd deploy
	# set the IMAGE_SECRET, IMAGE_NAME, IMAGE_TAG, SECRET_NAME to the right values on your case
	IMAGE_NAME=outscale/cloud-provider-osc && \
	IMAGE_TAG=v0.0.9beta && \
	SECRET_NAME=osc-secret && \
	helm uninstall k8s-osc-ccm 
	helm install k8s-osc-ccm deploy/k8s-osc-ccm \
		--set oscSecretName=$SECRET_NAME \
		--set image.repository=$IMAGE_NAME \
		--set image.tag=$IMAGE_TAG
		
	kubectl get pods -o wide -A -n kube-system | grep osc-cloud-controller-manager

```

