# 2048 game
 
This example creates a deployment for 2048 game exposed through a Service.

# Deploy

```bash
kubectl apply -f examples/2048/specs/
```

# Check

```
kubectl get all -n 2048-game
NAME                                   READY   STATUS    RESTARTS   AGE
pod/2048-deployment-595c9ff5f8-fw9wd   1/1     Running   0          84s
pod/2048-deployment-595c9ff5f8-w8hb9   1/1     Running   0          84s

NAME                   TYPE           CLUSTER-IP      EXTERNAL-IP                                                             PORT(S)          AGE
service/service-2048   LoadBalancer   10.32.108.201   ad4337ccf92644bc3ba83ee2d28dd9cf-588858383.eu-west-2.lbu.outscale.com   8383:30451/TCP   98s

NAME                              READY   UP-TO-DATE   AVAILABLE   AGE
deployment.apps/2048-deployment   2/2     2            2           84s

NAME                                         DESIRED   CURRENT   READY   AGE
replicaset.apps/2048-deployment-595c9ff5f8   2         2         2       84s
```

The url can be retrieved with `kubectl get svc -n 2048-game`.

# Cleanup

```
kubectl delete -f examples/2048/specs/
```