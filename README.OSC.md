- Prerequisites 
 -  You Must Tag all cluster resources (VPC, Instances, SG, ....) with the following tag
    The tag key = "OscK8sClusterID/" + clusterID
    The tag value is an ownership value 
     "shared" resource is shared between multiple clusters, and should not be destroyed
     "owned" the resource is considered owned and managed by the cluster
     
 ****Instances
 - You Must Tag all All cluster nodes with the following tag :
    Tag key "OscK8sNodeName"
    Tag values must be the k8s host name kubernetes.io/hostname 
 
 ****SG
 - The main SG to be applied to be attached/associated to the LBU must be Tagged with ("OscK8sMainSG/" + clusterID", True)
   Else an LB will be created and attached to all Nodes