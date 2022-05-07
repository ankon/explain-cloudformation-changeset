# explain-cloudformation-changeset

Tool to process a (possibly nested) CloudFormation changeset, and represent the changes in a human-understandable way. The main goal: 

_Make it possible to review a complex changeset and reason about the potential impacts of executing it._

## Building

```sh
go build
```

## Using

```sh
$ id=$(aws cloudformation create-change-set ... --output text --query Id)
$ ./explain-cloudformation-changeset --change-set-name=${id} --graph-output=graph.svg
```

The tool will download (nested) changeset descriptions and save them by default in the current working directory as JSON files. This can be changed by using the `--cache-dir` argument. If a changeset specified on the command-line already is cached, the cached version will be used. 

The examples can be used by setting the cache directory accordingly:

```sh
./explain-cloudformation-changeset --cache-dir=aws-examples --change-set-name=--change-set-name=SampleChangeSet-direct
```

## TODO & Ideas

* Table: Build a simple CSV with all planned changes
* Augment information from changeset with information from template(s) (should point to S3 location of packaged template, so we can find nested stack templates automatically)

## License

```license
Copyright 2022 Andreas Kohn <andreas.kohn@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
```

Note: The files in [aws-examples] have been taken from <https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/using-cfn-updating-stacks-changesets-samples.html> and are licensed under a [modified MIT license](https://github.com/awsdocs/aws-cloudformation-user-guide/blob/main/LICENSE-SAMPLECODE).