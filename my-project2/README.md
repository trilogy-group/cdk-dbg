# Welcome to your CDK TypeScript project

This is a blank project for CDK development with TypeScript.

The `cdk.json` file tells the CDK Toolkit how to execute your app.

## Useful commands

* `npm run build`   compile typescript to js
* `npm run watch`   watch for changes and compile
* `npm run test`    perform the jest unit tests
* `cdk deploy`      deploy this stack to your default AWS account/region
* `cdk diff`        compare deployed stack with current state
* `cdk synth`       emits the synthesized CloudFormation template


### Steps to get the useful data
1. go inside the  my-project2 folder. run `cd my-project2` in one terminal
2. run  `cdk --app "npx --node-arg='--inspect=2334' ts-node --prefer-ts-exts bin/my-project2.ts" synth`
3. Open another terminal
4. Go inside go-proj i.e run `cd go-proj`
5. run `go run main.go >  result.txt`
(Order must be this for proper running as debugger server needs to be started first and then debugger client should be connected to it)
This will produce a result.txt with all the data form the debugger
the data of importanse is inside `resourceIdToLocation map` , its values are printed against `FINAL MAPPINGS` in result.txt