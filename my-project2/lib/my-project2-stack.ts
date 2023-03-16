import { Stack, StackProps } from 'aws-cdk-lib';
import { Construct } from 'constructs';
import { aws_s3 as s3 } from 'aws-cdk-lib';
import { SqsToLambda } from "@aws-solutions-constructs/aws-sqs-lambda";
import * as lambda from 'aws-cdk-lib/aws-lambda';

// import * as sqs from 'aws-cdk-lib/aws-sqs';

export class MyProject2Stack extends Stack {
  constructor(scope: Construct, id: string, props?: StackProps) {
    super(scope, id, props);

    // const importedBucketFromArn = s3.Bucket.fromBucketArn(
    //   this,
    //   'imported_Arn_bucket',
    //   'arn:aws:s3:::my-tf-bucket-cloudfixlinter20230214182051614500000004',
    // );

    const PBucket = new s3.Bucket(this,"bucket_created_in_P")
    console.log("IN STACK DEC")

    new s3.Bucket(this,"Bucket2_in_P")
    
    // new SqsToLambda(this, 'SqsToLambdaPattern', {
    //   lambdaFunctionProps: {
    //     runtime: lambda.Runtime.NODEJS_14_X,
    //     handler: 'index.handler',
    //     code: lambda.Code.fromAsset(`lambda`)
    //   }
    // });

    // The code that defines your stack goes here

    // example resource
    // const queue = new sqs.Queue(this, 'MyProject2Queue', {
    //   visibilityTimeout: cdk.Duration.seconds(300)
    // });
  }
}
