# Frontend API retry policy

The shared frontend API clients retry `GET` requests up to three times when a
network failure occurs or the response is `502`, `503`, or `504`. Retries wait
300 ms and then 600 ms. Other HTTP responses, including validation, permission,
and rate-limit errors, are returned without retrying. Aborted requests are not
retried.

Login is the only current `POST` explicitly retried for transient failures to
recover from API cold starts. The sign-in form also prevents overlapping
submissions from triggering more than one login flow.

Other state-changing requests are sent once. A timeout or lost response does
not prove that the backend failed to apply a mutation, so automatically
replaying those requests could duplicate their effects. Do not enable retries
for additional mutations until the backend supports durable idempotency keys
and response replay for those operations.
