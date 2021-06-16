# Annotation

The Service for load balancer type supported annotation are :

| Annotation | Description |
| --- | --- |
| service.beta.kubernetes.io/aws-load-balancer-internal | the annotation used on the service to indicate that we want an internal ELB. |
| service.beta.kubernetes.io/aws-load-balancer-proxy-protocol | the annotation used on the service to enable the proxy protocol on an ELB. Right now we only accept the value "*" which means enable the proxy protocol on all ELB backends. In the future we could adjust this to allow setting the proxy protocol only on certain backends. |
| service.beta.kubernetes.io/aws-load-balancer-access-log-emit-interval | the annotation used to specify access log emit interval. |
| service.beta.kubernetes.io/aws-load-balancer-access-log-enabled | the annotation used on the service to enable or disable access logs. |
| service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-name | the annotation used to specify access log s3 bucket name. |
| service.beta.kubernetes.io/aws-load-balancer-access-log-s3-bucket-prefix | the annotation used to specify access log s3 bucket prefix. |
| service.beta.kubernetes.io/aws-load-balancer-connection-draining-enabled | the annnotation used on the service to enable or disable connection draining. |
| service.beta.kubernetes.io/aws-load-balancer-connection-draining-timeout | the annotation used on the service to specify a connection draining timeout. |
| service.beta.kubernetes.io/aws-load-balancer-connection-idle-timeout | the annotation used on the service to specify the idle connection timeout. |
| service.beta.kubernetes.io/aws-load-balancer-cross-zone-load-balancing-enabled | the annotation used on the service to enable or disable cross-zone load balancing. |
| service.beta.kubernetes.io/aws-load-balancer-extra-security-groups | the annotation used on the service to specify additional security groups to be added to ELB created |
| service.beta.kubernetes.io/aws-load-balancer-security-groups | the annotation used on the service to specify the security groups to be added to ELB created. Differently from the annotation  "service.beta.kubernetes.io/aws-load-balancer-extra-security-groups", this replaces all other security groups previously assigned to the ELB. |
| service.beta.kubernetes.io/aws-load-balancer-ssl-cert | the annotation used on the service to request a secure listener. Value is a valid certificate ARN. For more, see http://docs.aws.amazon.com/ElasticLoadBalancing/latest/DeveloperGuide/elb-listener-config.html CertARN is an IAM or CM certificate ARN, e.g. arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012 |
| service.beta.kubernetes.io/aws-load-balancer-ssl-ports | the annotation used on the service to specify a comma-separated list of ports that will use SSL/HTTPS listeners. Defaults to '*' (all). |
| service.beta.kubernetes.io/aws-load-balancer-ssl-negotiation-policy  | the annotation used on the service to specify a SSL negotiation settings for the HTTPS/SSL listeners of your load balancer. Defaults to AWS's default |
| service.beta.kubernetes.io/aws-load-balancer-backend-protocol | the annotation used on the service to specify the protocol spoken by the backend (pod) behind a listener. If `http` (default) or `https`, an HTTPS listener that terminates the connection and parses headers is created. If set to `ssl` or `tcp`, a "raw" SSL listener is used. If set to `http` and `aws-load-balancer-ssl-cert` is not used then a HTTP listener is used. |
| service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags | the annotation used on the service to specify a comma-separated list of key-value pairs which will be recorded as additional tags in the ELB. For example: "Key1=Val1,Key2=Val2,KeyNoVal1=,KeyNoVal2" |
| service.beta.kubernetes.io/aws-load-balancer-healthcheck-healthy-threshold | the annotation used on the service to specify the number of successive successful health checks required for a backend to be considered healthy for traffic. |
| service.beta.kubernetes.io/aws-load-balancer-healthcheck-unhealthy-threshold | the annotation used on the service to specify the number of unsuccessful health checks required for a backend to be considered unhealthy for traffic |
| service.beta.kubernetes.io/aws-load-balancer-healthcheck-timeout | is the annotation used on the service to specify, in seconds, how long to wait before marking a health check as failed. |
| service.beta.kubernetes.io/aws-load-balancer-healthcheck-interval | the annotation used on the service to specify, in seconds, the interval between health checks. |
| service.beta.kubernetes.io/osc-load-balancer-name-length | the annotation used on the service to specify, the load balancer name length max value is 32. |
| service.beta.kubernetes.io/osc-load-balancer-name | the annotation used on the service to specify, the load balancer name max length is 32 else it will be truncated. |

