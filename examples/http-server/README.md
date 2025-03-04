# HTTP-Server with Coraza

This example is intended to provide a straightforward way to spin up Coraza and grasp its behaviour.

## Run the example

```bash
go run . 
```

The server will be reachable at `http://localhost:8090`.

```bash
# True positive request (403 Forbidden)
curl -i 'localhost:8090/hello?id=0'
# True negative request (200 OK)
curl -i 'localhost:8090/hello'
```

You can customise the rules to be used by using the `DIRECTIVES_FILE` environment variable to load a directives file:

```bash
DIRECTIVES_FILE=my_directives.conf go run . 
```

You can also customise response body and response headers by using `RESPONSE_HEADERS` and `RESPONSE_BODY` environment variables respectively:

```bash
RESPONSE_BODY=creditcard go run . 
```

And then

```bash
# True positive request (403 Forbidden) due to matching response body
curl -i 'localhost:8090/hello'
```

## Customize WAF rules

The configuration of the WAF is provided directly inside the code under [main.go](https://github.com/corazawaf/coraza/blob/v3/dev/examples/http-server/main.go#L35). Feel free to play with it.

## Customize server behaviour

Customizing as shown below the [interceptor logic](https://github.com/corazawaf/coraza/blob/v3/dev/http/interceptor.go#L33), it is possible to make the example capable of echoing the body request. It comes in handy for testing rules that match the response body.

```go
func (i *rwInterceptor) Write(b []byte) (int, error) {
 buf := new(bytes.Buffer)
 reqReader, err := i.tx.RequestBodyReader()
 if err == nil {
     _, er := buf.ReadFrom(reqReader)
   if er == nil {
    b = append(b, buf.Bytes()...)
   }
 }
 return i.tx.ResponseBodyWriter().Write(b)
}
```
