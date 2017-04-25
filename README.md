Squirrel
===========

Squirrel is a wireless emulator for testing real-world software components above MAC layer.

This software was created for my [dissertation](https://song.gao.io/dissertation/). It's a userspace implementation with user and kernel context switches for every packet. While Squirrel is too slow for emulating late cues and only deals with packet loss, it takes wireless congestions into consideration and can emulate CSMA/CA. For one node this wouldn't make a difference, but for multiple nodes where interference is a problem, it's a bit more realistic than just setting arbitrary packet loss.

