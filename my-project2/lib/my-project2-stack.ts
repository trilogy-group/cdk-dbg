import { Stack, StackProps, Duration } from 'aws-cdk-lib';
import { Construct } from 'constructs';
import { aws_s3 as s3 } from 'aws-cdk-lib';
import { EventbridgeToStepfunctions, EventbridgeToStepfunctionsProps } from '@aws-solutions-constructs/aws-eventbridge-stepfunctions';
import * as stepfunctions from 'aws-cdk-lib/aws-stepfunctions';
import * as events from 'aws-cdk-lib/aws-events';

// import * as sqs from 'aws-cdk-lib/aws-sqs';

export class MyProject2Stack extends Stack {
  constructor(scope: Construct, id: string, props?: StackProps) {
    super(scope, id, props);

    const importedBucketFromArn = s3.Bucket.fromBucketArn(
      this,
      'imported_Arn_bucket',
      'arn:aws:s3:::my-tf-bucket-cloudfixlinter20230214182051614500000004',
    );

    const PBucket = new s3.Bucket(this,"bucket_created_in_P")
    console.log("IN STACK DEC")
    const startState = new stepfunctions.Pass(this, 'StartState');

    new EventbridgeToStepfunctions(this, 'test-eventbridge-stepfunctions-stack', {
      stateMachineProps: {
        definition: startState
      },
      eventRuleProps: {
        schedule: events.Schedule.rate(Duration.minutes(5))
      }
    });

    // The code that defines your stack goes here

    // example resource
    // const queue = new sqs.Queue(this, 'MyProject2Queue', {
    //   visibilityTimeout: cdk.Duration.seconds(300)
    // });
  }
}
