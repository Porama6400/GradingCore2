#include<stdio.h>
#include<stdlib.h>
#include<string.h>

int main(){
   int len = 10000000;
   for(int i = 0; i < len; i++){
	char *p = malloc(sizeof(char));
	if(p == 0){
	    printf("%d\n", i);
	}
   }
   printf("Done\n");
   exit(0);
}
