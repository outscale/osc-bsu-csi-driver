#!/bin/sh
set -e

clean_status()
{
  kubectl delete -f ./examples/simple-lb/specs/ 2> /dev/null || true
}

kubectl create namespace simple-lb || true
kubectl apply  -f examples/simple-lb/specs/

for i in {1..30}; do
  deploy_count=`kubectl get deploy -n simple-lb | tail -1 | awk '{print $2}'`
  if [ "$deploy_count" == "1/1" ]; then
      break
  elif [ $i -eq 30 ]; then
      echo "Create Deployment fails"
      clean_status
      exit 1
  fi
  echo "Wait for Deployment"
  sleep 10
done

for i in {1..30}; do
  svc_host=`kubectl get svc -n simple-lb | tail -1 | awk '{print $4}'`
  if  [[ "$svc_host" == *".eu-west-2.lbu.outscale.com" ]]; then
      break
  elif [ $i -eq 30 ]; then
      echo "Create Service LB fails"
      clean_status
      exit 1
  fi
  echo "Wait for LB"
  sleep 10
done

svc_host=`kubectl get svc -n simple-lb | tail -1 | awk '{print $4}'`
for i in {1..30}; do
  status=`curl $svc_host`
  if [ "$?" == "0" ]; then
      break
  elif [ $i -eq 20 ]; then
      echo "Service Respond fails"
      clean_status
      exit 1
  fi
  echo "Wait Service Respond"
  sleep 10
done

clean_status
echo "Pass"
exit 0
