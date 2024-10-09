Reader/write support with buffer in RAM. If more than the declared amount is required, disk memory is used as a buffer. Process files up to hundreds of gigabytes in size without much effort and loading everything into memory.


### Example

Look into filebuf_test.go

```go
    //  if you exceed 1024 bytes, store everything else on disk
	buf := filebuf.New(1024, true, false)
```