<template>
  <div class="resultContainer container is-fullhd" v-if="usecases.mix!=null">
  <p></p>
  <p class="logoColor title is-size-5" style="padding-bottom:5px">Example Use Cases</p>  

  <div v-for="(catcases,index) in usecases" class="columns">
    <div class="column"></div>
    <div class="tags column is-four-fifths" >
        <a class="is-link" @click="execCatUseCases(catcases)">{{index}} <template v-if="index!='mix'">centric</template>  examples</a>
        <a v-for="(usecase,index2) in catcases" class="tag" @click="execUseCase(usecase)">{{usecase.name}}</a>
    </div>
    <div class="column"></div>    
  </div>
  <!-- <p class="title is-size-6">more example...</p> -->
  </div>
</template>

<script>
export default {
  name: "Usecase",
  props: {
    usecases: {
      type: Object,
      required: true
    }
  },
  methods: {
    exampleQuery: function (query) {
      // trigger a usecase query

    },
    execUseCase: function (usecase) {
      this.$emit("usecase", usecase);
    },
    execCatUseCases: function (catusecases) {
      this.$emit("catusecases", catusecases);
    }
  }
};
</script>
 
<style scoped>
a[target^="_blank"]:after {
  /** content: url(http://upload.wikimedia.org/wikipedia/commons/6/64/Icon_External_Link.png);**/
  content: url(data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAoAAAAKCAYAAACNMs+9AAAAVklEQVR4Xn3PgQkAMQhDUXfqTu7kTtkpd5RA8AInfArtQ2iRXFWT2QedAfttj2FsPIOE1eCOlEuoWWjgzYaB/IkeGOrxXhqB+uA9Bfcm0lAZuh+YIeAD+cAqSz4kCMUAAAAASUVORK5CYII=);
  margin: 0 0 0 1px;
}
</style>
