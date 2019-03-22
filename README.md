# git-dropbox

Simulate `hg largefile` on git with dropbox

## Introduction

When we want to add large files into git repository, I wonder you do `git add` really.
`hg largefile` put commit hash value into the file. And store into `~/.cache/largefiles`.
This is a git implementation of `hg largefile` using golang and dropbox.
The files are stored in `~/.gitasset/data`.

If you want to checkout this repository on another environment, you must
 sync `~/.gitasset/data` before doing that.

## Instruction

### Install

    $ go get github.com/mattn/git-dropbox

### .gitconfig

Put follow into your `~/.gitconfig` or `.git/config`:

```
[filter "dropbox"]
    clean = git-dropbox store
    smudge = git-dropbox load
```

### .gitattribute

Create new file `.gitattribute` and put like follow:

```
*.png  filter=largefile
*.jpeg filter=largefile
*.jpg  filter=largefile
*.gif  filter=largefile
```

This mean which files are stored on your dropbox.

## Warning

Note that don't cheating, cracking, or any other corrections. This
application store application-token that i created. If you had corrections,
and dropbox offcial stop and block this token, all users will have to
replace new application token.

## Author

Yasuhiro Matsumoto (mattn.jp@gmail.com)

## Thanks

This is based on @methane's idea.

https://github.com/methane/git-largefiles

