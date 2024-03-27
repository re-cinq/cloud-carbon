# Sources

A "source" is a component where we ingest various metrics from. This allows us
to have a plugin like structure where we can pull data from various places

## AWS Source

You can configure AWS cloudwatch as a source by providing a credentials under
the AWS section in the [config](./config.md#example)

Please take note that this will incur costs

## Google Cloud Source

You will be able to use Google Clouds monitoring setup. This will pull metrics
per project and is configurable in the [config](./config.md#example) file


**Note on Credentials**

If you do not set any credentials the Google Cloud Provider will default to
`GOOGLE_APPLICATION_CREDENTIALS`
## Azure Source
**Coming Soon....**
