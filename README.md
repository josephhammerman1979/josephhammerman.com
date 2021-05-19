# squaremeet (owned by: Databases)

Don't have any opinions on how to lay out a project? Want to get started quickly?

Great, do the following:

* Clone the repo
* Run `./init.sh` and follow the prompts

## Take a penny, leave a penny

If you lived here, you'd be home already!

## Makefile Tasks Reference

### Do everything!

```
$ make
```

### Format the code

```
$ make fmt
```

### Test the code

```
$ make test
```

### Check the test coverage

```
$ make coverage
```

### Build the code

```
$ make build
```

> Note: There's also `make build-darwin` and `make build-linux` separately.

### Install the code

```
$ make install-darwin
```

OR

```
$ make install-linux
```

### Archive the code

```
$ make archive
```

### Build the Docker Container

```
$ make container
```

### Cleanup

```
$ make clean
```

### Tidy up dependencies

```
$ make tidy
```

### Fix go mod

Sometimes go mod gets in a fit, use this to try and fix it.

```
$ make gomod
```

Sometimes go mod _really_ gets in a fit, use this to try and fix everything.

```
$ make gomod-clean
```

### Publish to Artifactory

We typically use [semvar](https://semver.org/) when publishing tags and versions of go packages.

```
$ make publish
What version would you like to publish? v?.?.?
```
