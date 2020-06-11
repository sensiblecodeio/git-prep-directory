build:
	docker build -t git-prep-directory .
	docker run --rm git-prep-directory cat /go/bin/git-prep-directory > git-prep-directory
	chmod u+x git-prep-directory

clean:
	rm git-prep-directory

.PHONY: clean
