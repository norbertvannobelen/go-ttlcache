# ttl-cache alternative

This implementation has 2 differences compared to the package in the root:

* It has only one map to store the data in. Consequence is that lookup takes 1 step more (has to follow one more pointer). Difference due to this is about 1000ns per read;
* It checks the TTL exact, which slows down the processing with another 300ns/read (The exact expiration has not been tested in the root package since it would be twice as slow as the original root benchmark (2x the read for the same record). In case of expected exact expiration handling, this alternative package will be about 75% faster than the root package).
