# Multiple roles with Service Accounts without annotation


This folder is here to demonstrate how to set multiple roles without **annottation** on the service account but by looking for the AWS_ROLE_ARN as instumented in this PR.

To you can build your own webhook image or edit the deployment in the deploy folder to use my image I built from the included PR edits.

072792469044.dkr.ecr.eu-west-1.amazonaws.com/eks/pod-identity-webhook


## To Run


### Retrieve your EKS Cluster ID

Your cluster Id would be parsed from your https endpoint:

```bash
https://<This is your cluster ID>.aaa.eu-west-1.eks.amazonaws.com
```

### Create your roles

Use the roles file included in this folder. Replace the **CLUSTERID HERE** with the cluster ID. 

```bash
aws cloudformation create-stack --stack-name sa-roles-tests \
	--template-body file://roles.yaml --capabilities CAPABILITY_IAM \
	--parameters ParameterKey=ClusterID,ParameterValue=**CLUSTERID HERE** \
	ParameterKey=Namespace,ParameterValue=test-iam
```
From this template (included in folder)
```yaml
AWSTemplateFormatVersion: 2010-09-09
Parameters: 
  ClusterID: 
    Type: String
    Description: Enter your EKS OIDC Provider arn
    MinLength: 32
    MaxLength: 32
    AllowedPattern: ^[a-zA-Z0-9]*$
  Namespace: 
    Type: String
    Description: Enter the Namespace your pods belong to
Resources:
  EC2ReadRole:
    Type: 'AWS::IAM::Role'
    Properties:
      AssumeRolePolicyDocument:
        !Sub |
            {
              "Version": "2012-10-17",
              "Statement": [
                {
                  "Effect": "Allow",
                  "Principal": {
                    "Federated": "arn:aws:iam::${AWS::AccountId}:oidc-provider/oidc.eks.${AWS::Region}.amazonaws.com/id/${ClusterID}"
                  },
                  "Action": "sts:AssumeRoleWithWebIdentity",
                  "Condition": {
                    "StringEquals": {
                      "oidc.eks.${AWS::Region}.amazonaws.com/id/${ClusterID}:sub":"system:serviceaccount:${Namespace}:default"
                    }
                  }
                }
               ]
             }
      Path: /
      Policies:
        - PolicyName: ec2describe
          PolicyDocument:
            Version: 2012-10-17
            Statement:
              - Effect: Allow
                Action: 'ec2:DescribeInstances'
                Resource: '*'
  S3ReadBuckets:
    Type: 'AWS::IAM::Role'
    Properties:
      AssumeRolePolicyDocument:
        !Sub |
            {
              "Version": "2012-10-17",
              "Statement": [
                {
                  "Effect": "Allow",
                  "Principal": {
                    "Federated": "arn:aws:iam::${AWS::AccountId}:oidc-provider/oidc.eks.${AWS::Region}.amazonaws.com/id/${ClusterID}"
                  },
                  "Action": "sts:AssumeRoleWithWebIdentity",
                  "Condition": {
                    "StringEquals": {
                      "oidc.eks.${AWS::Region}.amazonaws.com/id/${ClusterID}:sub":"system:serviceaccount:${Namespace}:default"
                    }
                  }
                }
               ]
            }
      Path: /
      Policies:
        - PolicyName: s3list
          PolicyDocument:
            Version: 2012-10-17
            Statement:
              - Effect: Allow
                Action:
                  - 's3:List*'
                  - 's3:Get*'
                Resource: '*'
Outputs:
  EC2RoleARN:
    Value: !GetAtt EC2ReadRole.Arn
  S3RoleARN:
    Value: !GetAtt S3ReadBuckets.Arn

```



### Retrive your iam role arns.
```bash
aws cloudformation describe-stacks --stack-name sa-roles-tests --query 'Stacks[0].Outputs'
```

Replace your roles from the cloudformation output in the pod.yaml (included in the folder as well).  You can also replace your region.

```yaml
---
apiVersion: v1
kind: Pod
metadata:
  name: test-pod
  namespace: test-iam
  labels:
    name: test-pod
spec:
  securityContext:
    fsGroup: 1337
  containers:
    - name: s3
      image: ubuntu
      env:
        - name: AWS_ROLE_ARN
          value: #Put your S3 role here
        - name: AWS_DEFAULT_REGION
          value: eu-west-1
      command: ["/bin/bash","-c"]
      args:
        - apt-get update;
          apt-get install -y python3-pip;
          pip3 install --upgrade --user awscli;
          export PATH=$HOME/.local/bin:$PATH;
          aws s3api list-buckets;
          echo "The above command should return S3 buckets the below command should fail";
          aws ec2 describe-instances;
          exit
    - name: ec2
      image: ubuntu
      env:
        - name: AWS_ROLE_ARN
          value: #Put your EC2 role here
        - name: AWS_DEFAULT_REGION
          value: eu-west-1
      command: ["/bin/bash","-c"]
      args:
        - apt-get update;
          apt-get install -y python3-pip;
          pip3 install --upgrade --user awscli;
          export PATH=$HOME/.local/bin:$PATH;
          aws ec2 describe-instances;
          echo "The above command should return EC2 instances the below command should fail";
          aws s3api list-buckets;
          exit
```

### Run the pod

```bash
kubectl create ns test-iam
kubectl apply -f pod.yaml
```
### Follow the logs

Either in two terminals or one after the other.
```bash
kubectl logs -n test-iam test-pod s3 -f
kubectl logs -n test-iam test-pod ec2 -f
```
The EC2 container sghould list ec2 instances but not buckets.
The S3 container should list buickets but not ec2 instances.

### Tear down 
```
aws cloudformation delete-stack --stack-name sa-roles-tests
kubectl delete ns test-iam
```


