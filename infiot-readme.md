# Edgex-go repository
This repo is a fork of EdgeX foundry edgex-go repository, forked by Infiot. 


## How to start working with the repository
1. infiot-0.7.1 is the active branch used for development of Infiot 
   relevant features. Currently this is not getting merged into the master.
   So use this branch as the base for feature development rather than master

2. There are multiple ways to compile. If you want to use Infiot dev 
   environment and scripts, one approach is to modify the edgex 
   core services makefile (coredgsvc.mk) to make a clone of 
   edgex-go from the local repo/directory that has your changes. Once this
   is done, coreedgsvc make operations will take your changes and can be 
   built/tested.

3. To checkin using standard 2FA authentication, ensure that git repo URL 
   uses SSH.  Use git remote command to check, it should show the following
   output. 
   $ git remote -v
   origin	git@github.com:infiotinc/edgex-go.git (fetch)
   origin	git@github.com:infiotinc/edgex-go.git (push)

   If not, use the following command to modify the URL 
   $ git remote set-url origin git@github.com:infiotinc/edgex-go.git

   Once this is done, you will be able to check-in to the Edgex-go repo.

