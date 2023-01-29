# Go-Matchmaker Documentation

## Algorithm

Pseudocode algorithm for API and Maker services

### API service

```bash
get request
apply auth middleware
# set status to OCCUPIED to avoid race condition with other requests from client
# use redis SET operation with GET argument, GETSET is deprecated
# used here for simplicity
getset client request from redis, set status to OCCUPIED

switch request.status:
    case no request:
    case FAILED:
        # no request found or last request is FAILED
        update request status to CREATED
        # requestID is clientID
        push requestID to Maker message queue
        respond with 202
    case CREATED:
    case IN_PROGRESS:
    case OCCUPIED:
        # request in progress, no need for a new one
        respond with 202
    case DONE:
        # last request DONE, but container can be no longer waiting
        url = hostname:port from request
        result, err = send GET to url/reservation/{client-id}
        
        if err == nil && result == 200:
            # remove occupied to allow new requests from client
            update request status to DONE
            respond with 200, hostname:{exposed-port}
        else:
            update request status to CREATED
            # requestID is clientID
            push requestID to Maker message queue
            respond with 202
```

### Maker service

```bash
create MAX_CONCURRENT_JOBS goroutines
    # each goroutine
    for true:
        request = blocking pop on message queue
        update request status to IN_PROGRESS
        for each running container:
            # request-id is client-id
            url = container.hostname:port
            result = send POST to url/reservation/{request-id}
            if result == 200:
                update request hostname to container.hostname
                update request status to DONE
                return
        
        # no available running containers, need new one
        if mutex.tryLock:
            if any exited container:
                start container
                expose port
                url = container.hostname:port
                result = send POST to url/reservation/{request-id}
                if result == 200:
                    update request hostname to container.hostname
                    update request status to DONE
                    return
                else:
                    # something wrong with container
                    # new container should be available for reservation
                    fatal
            
            # no exited containers, start new one
            create container
            start container
            expose port
            url = container.hostname:port
            result = send POST to url/reservation/{request-id}
            if result == 200:
                update request hostname to container.hostname
                update request status to DONE
                return
            else:
                # something wrong with container
                # new container should be available for reservation
                fatal
                
        # we tried to lock mutex, wait if it's locked and start search from the begin
        else:
            sleep LOOKUP_COOLDOWN
```
