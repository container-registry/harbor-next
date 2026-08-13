# Harbor on AWS: Zero-Static-Secret Deployment

Deploy Harbor on EKS with native AWS authentication: **RDS IAM Database Authentication** and **S3 IRSA**.

For the full walkthrough, see the blog post:
[Hardening Harbor on AWS: Achieving Zero-Static-Secret Architecture](https://container-registry.com/posts/hardening-harbor-on-aws/)

## Prerequisites

- AWS Account with IAM permissions
- `aws-cli`, `kubectl`, `helm`, `eksctl` installed
- Access to 8gears Container Registry

## Quick Start

### 1. Set Environment Variables

```bash
export AWS_REGION="us-east-1"
export AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
export CLUSTER_NAME="harbor-cluster"
export BUCKET_NAME="harbor-registry-store"
export SA_NAME="harbor-sa"
export NAMESPACE="harbor"
export DB_NAME="registry"
export DB_USER="harbor_iam_user"
export DB_INSTANCE_ID="harbor-db"
```

### 2. Create EKS Cluster with OIDC

```bash
eksctl create cluster \
  --name $CLUSTER_NAME \
  --region $AWS_REGION \
  --version 1.30 \
  --with-oidc \
  --managed \
  --node-type t3.medium \
  --nodes 2
```

### 3. Create S3 Bucket

```bash
aws s3 mb "s3://$BUCKET_NAME" --region $AWS_REGION
```

### 4. Create RDS Instance

Create the database first so its resource ID is available for the IAM policy.
Put it in the same VPC as the EKS cluster, in at least two subnets, and open
TCP/5432 from the cluster's node security group.

```bash
# Reuse the cluster's VPC + subnets:
export VPC_ID=$(aws eks describe-cluster --name $CLUSTER_NAME --region $AWS_REGION \
  --query "cluster.resourcesVpcConfig.vpcId" --output text)
export SUBNET_IDS=$(aws ec2 describe-subnets --region $AWS_REGION \
  --filters "Name=vpc-id,Values=$VPC_ID" \
  --query "Subnets[].SubnetId" --output text | tr '\t' ' ')
export NODE_SG=$(aws eks describe-cluster --name $CLUSTER_NAME --region $AWS_REGION \
  --query "cluster.resourcesVpcConfig.clusterSecurityGroupId" --output text)

# DB subnet group (≥2 subnets in ≥2 AZs):
aws rds create-db-subnet-group \
  --db-subnet-group-name ${CLUSTER_NAME}-subnets \
  --db-subnet-group-description "Harbor RDS" \
  --subnet-ids $SUBNET_IDS --region $AWS_REGION

# Security group that allows the EKS node SG to reach 5432:
export DB_SG=$(aws ec2 create-security-group \
  --group-name ${CLUSTER_NAME}-db-sg \
  --description "Harbor RDS from EKS" \
  --vpc-id $VPC_ID --region $AWS_REGION --query 'GroupId' --output text)
aws ec2 authorize-security-group-ingress \
  --group-id $DB_SG --protocol tcp --port 5432 \
  --source-group $NODE_SG --region $AWS_REGION

aws rds create-db-instance \
    --db-instance-identifier $DB_INSTANCE_ID \
    --db-instance-class db.t3.medium \
    --engine postgres \
    --engine-version 18.1 \
    --master-username harbor_admin \
    --master-user-password "<yourPassword>" \
    --allocated-storage 20 \
    --db-name $DB_NAME \
    --enable-iam-database-authentication \
    --vpc-security-group-ids $DB_SG \
    --db-subnet-group-name ${CLUSTER_NAME}-subnets \
    --no-publicly-accessible \
    --region $AWS_REGION

echo "Waiting for RDS instance (5-10 minutes)..."
aws rds wait db-instance-available \
  --db-instance-identifier $DB_INSTANCE_ID \
  --region $AWS_REGION

export DB_ENDPOINT=$(aws rds describe-db-instances \
    --db-instance-identifier $DB_INSTANCE_ID \
    --region $AWS_REGION \
    --query "DBInstances[0].Endpoint.Address" \
    --output text)

export DB_RESOURCE_ID=$(aws rds describe-db-instances \
    --db-instance-identifier $DB_INSTANCE_ID \
    --region $AWS_REGION \
    --query "DBInstances[0].DbiResourceId" \
    --output text)
```

### 5. Create IAM Policy

The `rds-db:connect` resource is scoped to the specific DB instance using its `DbiResourceId`.

```bash
aws iam create-policy \
    --policy-name HarborNativePolicy \
    --policy-document '{
        "Version": "2012-10-17",
        "Statement": [
            {
                "Effect": "Allow",
                "Action": [
                    "s3:GetObject", "s3:PutObject", "s3:DeleteObject",
                    "s3:ListBucket", "s3:GetBucketLocation",
                    "s3:ListBucketMultipartUploads",
                    "s3:AbortMultipartUpload", "s3:ListMultipartUploadParts"
                ],
                "Resource": [
                    "arn:aws:s3:::'"$BUCKET_NAME"'",
                    "arn:aws:s3:::'"$BUCKET_NAME"'/*"
                ]
            },
            {
                "Effect": "Allow",
                "Action": ["rds-db:connect"],
                "Resource": [
                    "arn:aws:rds-db:'"$AWS_REGION"':'"$AWS_ACCOUNT_ID"':dbuser:'"$DB_RESOURCE_ID"'/'"$DB_USER"'"
                ]
            }
        ]
    }'
```

### 6. Create IRSA Service Account

This creates a Kubernetes ServiceAccount in the `harbor` namespace with the
`eks.amazonaws.com/role-arn` annotation — all Harbor components in the values
file reference this same SA so they all use the same IAM role.

```bash
eksctl create iamserviceaccount \
  --cluster=$CLUSTER_NAME --region=$AWS_REGION \
  --name=$SA_NAME --namespace=$NAMESPACE \
  --attach-policy-arn="arn:aws:iam::$AWS_ACCOUNT_ID:policy/HarborNativePolicy" \
  --approve
```

### 7. Configure IAM Database User

Connect to RDS from inside the cluster (no need to expose RDS publicly) and
create the IAM-enabled user:

```bash
kubectl -n $NAMESPACE run psql --rm -i --image=postgres:16 --restart=Never \
  --env PGPASSWORD="<yourMasterPassword>" \
  --command -- psql -h $DB_ENDPOINT -U harbor_admin -d $DB_NAME -v ON_ERROR_STOP=1 <<SQL
CREATE USER harbor_iam_user WITH LOGIN;
GRANT rds_iam TO harbor_iam_user;
GRANT ALL PRIVILEGES ON DATABASE registry TO harbor_iam_user;
GRANT ALL ON SCHEMA public TO harbor_iam_user;
SQL
```

### 8. Deploy Harbor

Edit `values-aws-native.yaml` to substitute the five placeholders (DB endpoint,
region, bucket, image registry, image tag), then install from the `harbor-next`
chart:

```bash
sed -i "s|<YOUR_DB_ENDPOINT>|$DB_ENDPOINT|g;
        s|<YOUR_AWS_REGION>|$AWS_REGION|g;
        s|<YOUR_BUCKET_NAME>|$BUCKET_NAME|g" values-aws-native.yaml
# Also set <YOUR_IMAGE_REGISTRY> and <YOUR_IMAGE_TAG> manually.

helm upgrade --install my-harbor \
  oci://8gears.container-registry.com/8gcr/charts/harbor-next \
  --version 3.0.0 \
  --namespace $NAMESPACE --create-namespace \
  -f values-aws-native.yaml
```

## Verification

```bash
# 1. Pods up
kubectl -n $NAMESPACE get pods

# 2. IAM auth activated in core
kubectl -n $NAMESPACE logs deploy/my-harbor-core --tail=200 \
  | grep -E 'IAM Auth|migrated successfully'
# Expect:
#   IAM Auth: Enabled for region=... endpoint=...:5432 user=harbor_iam_user
#   IAM Auth: Token generated for database migration
#   The database has been migrated successfully

# 3. S3 IRSA wired to registry pod
kubectl -n $NAMESPACE get pod -l app.kubernetes.io/component=registry -o yaml \
  | grep -E 'AWS_ROLE_ARN|AWS_WEB_IDENTITY'

# 4. End-to-end: push an image and confirm S3 objects + DB rows
kubectl -n $NAMESPACE port-forward svc/my-harbor-core 18080:80 &
echo 'Harbor12345' | docker login localhost:18080 -u admin --password-stdin
docker tag alpine:3.19 localhost:18080/library/alpine:3.19
docker push localhost:18080/library/alpine:3.19
aws s3 ls s3://$BUCKET_NAME/docker/registry/v2/ --recursive | head
curl -s -u admin:Harbor12345 \
  http://localhost:18080/api/v2.0/projects/library/repositories/alpine/artifacts | head -c 200
```

The chart doesn't ship an Ingress by default. For external access, set
`ingress.enabled: true` in the values file and install an Ingress controller
(ALB, nginx, etc.) that routes to the `my-harbor-core` service.

## Environment Variables Reference

| Variable | Description |
|---|---|
| `POSTGRESQL_USE_IAM_AUTH` | Set to `"true"` to enable RDS IAM authentication |
| `POSTGRESQL_AWS_REGION` | AWS region for token generation (falls back to `AWS_REGION`) |
| `POSTGRESQL_HOST` | RDS endpoint |
| `POSTGRESQL_PORT` | RDS port (default: 5432) |
| `POSTGRESQL_USERNAME` | IAM-enabled database user |

## Cleanup

```bash
helm uninstall my-harbor -n $NAMESPACE
kubectl delete namespace $NAMESPACE
aws rds delete-db-instance --db-instance-identifier $DB_INSTANCE_ID --skip-final-snapshot --region $AWS_REGION
aws s3 rm s3://$BUCKET_NAME --recursive && aws s3 rb s3://$BUCKET_NAME
eksctl delete cluster --name $CLUSTER_NAME --region $AWS_REGION
```
